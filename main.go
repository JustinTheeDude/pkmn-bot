// Command pokebot-go is a fast concurrent Pokemon Center restock monitor.
//
// It polls Pokemon Center product pages and fires instant alerts
// (Discord webhook + browser auto-open) when sealed product looks in-stock.
//
// NOTE on "auto-checkout": pokemoncenter.com sits behind Queue-it virtual
// queue + Akamai/Incapsula bot mitigation (plain HTTP GETs get 403).
// Fully unattended checkout requires a real logged-in browser session,
// passing the queue manually, plus residential proxies. This bot does the
// part that actually wins drops: sub-second detection + instant alert +
// one-key cart link. The --auto-open flag opens the product the moment it
// flips so you can smash checkout in your already-logged-in browser.
//
// Usage:
//
//	go run . --config config.json
//	go run . --config config.json --once   # single check, good for testing
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Config is loaded from config.json.
type Config struct {
	CheckIntervalSec int            `json:"check_interval_sec"`
	JitterMs         int            `json:"jitter_ms"`
	TimeoutSec       int            `json:"timeout_sec"`
	DiscordWebhook   string         `json:"discord_webhook"`
	AutoOpenBrowser  bool           `json:"auto_open_browser"`
	BeepOnHit        bool           `json:"beep_on_hit"`
	Discover         DiscoverConfig `json:"discover"`
	Products         []Product      `json:"products"`
}

// Product is one sealed item to watch.
type Product struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	MaxPrice   string `json:"max_price_note,omitempty"`
	NotifyOnly bool   `json:"notify_only"`
}

var userAgents = []string{
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36 Edg/125.0.0.0",
}

func main() {
	cfgPath := flag.String("config", "config.json", "path to config.json")
	profilePath := flag.String("profile", "profile.json", "path to profile.json (0600, local only)")
	uiAddr := flag.String("ui", "", "serve local dashboard, e.g. --ui 127.0.0.1:8080 (localhost only)")
	checkoutIdx := flag.Int("checkout", -1, "run automated browser checkout for product index N (default dry-run)")
	placeOrder := flag.Bool("place-order", false, "with --checkout: actually click Place Order (spends money)")
	headed := flag.Bool("headed", true, "with --checkout: visible browser window (recommended for queue/CAPTCHA)")
	once := flag.Bool("once", false, "check every product once and exit")
	flag.Parse()

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("load config %s: %v", *cfgPath, err)
	}
	if len(cfg.Products) == 0 {
		log.Fatal("no products in config.json — add at least one URL")
	}

	log.Printf("pokebot-go: watching %d product(s) every ~%ds (Go, concurrent)", len(cfg.Products), cfg.CheckIntervalSec)
	setUIStatus(len(cfg.Products), cfg.CheckIntervalSec, "live")

	if *checkoutIdx >= 0 {
		if *checkoutIdx >= len(cfg.Products) {
			log.Fatalf("product index %d out of range (0-%d)", *checkoutIdx, len(cfg.Products)-1)
		}
		p, _ := loadProfile(*profilePath)
		res := RunCheckout(CheckoutOptions{
			ProductURL: cfg.Products[*checkoutIdx].URL,
			Profile:    p,
			Headed:     *headed,
			PlaceOrder: *placeOrder,
		})
		for _, s := range res.Steps {
			fmt.Println(" -", s)
		}
		if res.Error != "" {
			log.Fatalf("checkout failed: %s", res.Error)
		}
		return
	}

	if *uiAddr != "" {
		// Run monitor workers in background, UI in foreground.
		for i, p := range cfg.Products {
			go func(idx int, pr Product) {
				time.Sleep(time.Duration(idx*400) * time.Millisecond)
				tickerJitterLoop(pr, cfg)
			}(i, p)
		}
		if cfg.Discover.Enabled {
			go discoverLoop(&cfg, *cfgPath, *profilePath)
		}
		// Touch profile file so perms note is visible early (0600).
		if _, err := loadProfile(*profilePath); err != nil {
			log.Printf("profile: %v", err)
		}
		if err := startUIServer(*uiAddr, *cfgPath, *profilePath, &cfg); err != nil {
			log.Fatalf("UI server: %v", err)
		}
		return
	}

	if *once {
		var wg sync.WaitGroup
		for _, p := range cfg.Products {
			wg.Add(1)
			go func(pr Product) {
				defer wg.Done()
				checkOne(pr, cfg, true)
			}(p)
		}
		wg.Wait()
		return
	}

	var wg sync.WaitGroup
	for i, p := range cfg.Products {
		wg.Add(1)
		go func(idx int, pr Product) {
			defer wg.Done()
			// Stagger workers so N products don't fire at the exact same ms.
			time.Sleep(time.Duration(idx*400) * time.Millisecond)
			tickerJitterLoop(pr, cfg)
		}(i, p)
	}
	if cfg.Discover.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			discoverLoop(&cfg, *cfgPath, *profilePath)
		}()
	}
	wg.Wait()
}

func loadConfig(path string) (Config, error) {
	c := Config{
		CheckIntervalSec: 20,
		TimeoutSec:       15,
		Discover:         defaultDiscover(),
	}
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()
	// Decode over defaults so absent keys keep sane values; then fix
	// nested structs encoding/json leaves half-zeroed.
	var raw map[string]json.RawMessage
	data, err := io.ReadAll(f)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, err
	}
	if _, ok := raw["discover"]; !ok {
		c.Discover = defaultDiscover()
	} else {
		normalizeDiscover(&c.Discover)
	}
	if c.CheckIntervalSec <= 0 {
		c.CheckIntervalSec = 20
	}
	if c.TimeoutSec <= 0 {
		c.TimeoutSec = 15
	}
	if c.JitterMs < 0 {
		c.JitterMs = 0
	}
	return c, nil
}

func saveConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func tickerJitterLoop(p Product, cfg Config) {
	for {
		checkOne(p, cfg, false)
		sleep := time.Duration(cfg.CheckIntervalSec)*time.Second + time.Duration(randMs(cfg.JitterMs))*time.Millisecond
		time.Sleep(sleep)
	}
}

// randMs returns 0..n ms pseudo-random using time nanos (no extra deps).
func randMs(n int) int {
	if n <= 0 {
		return 0
	}
	return int(time.Now().UnixNano()/1e6) % (n + 1)
}

// CheckResult summarises one poll.
type CheckResult struct {
	InStock   bool
	Blocked   bool // bot mitigation / queue page
	Status    int
	Note      string
	LatencyMs int64
}

func checkOne(p Product, cfg Config, verbose bool) {
	client := newClient(cfg.TimeoutSec)
	start := time.Now()
	res, body := fetchProduct(client, p.URL)
	elapsed := time.Since(start).Milliseconds()
	res.LatencyMs = elapsed

	ts := time.Now().Format("15:04:05")
	switch {
	case res.Blocked:
		log.Printf("[%s] QUEUED/BLOCKED %-40s http=%d %dms :: %s", ts, trunc(p.Name, 40), res.Status, elapsed, res.Note)
		if verbose {
			fmt.Printf("  -> %s : blocked (%s). This is normal for Pokemon Center (Queue-it/Akamai). Keep your browser logged in; the bot will still catch the flip.\n", p.Name, res.Note)
		}
	case res.InStock:
		log.Printf("[%s] IN-STOCK!!! %-40s %dms :: %s", ts, trunc(p.Name, 40), elapsed, p.URL)
		alert(p, cfg, res.Note)
	default:
		log.Printf("[%s] OOS        %-40s %dms", ts, trunc(p.Name, 40), elapsed)
		if verbose {
			_ = body // reserved for --dump debugging
		}
	}
}

func newClient(timeoutSec int) *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Timeout: time.Duration(timeoutSec) * time.Second, Jar: jar}
}

func fetchProduct(client *http.Client, url string) (CheckResult, string) {
	ua := userAgents[int(time.Now().UnixNano())%len(userAgents)]
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return CheckResult{Note: "bad request: " + err.Error()}, ""
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Referer", "https://www.pokemoncenter.com/")

	resp, err := client.Do(req)
	if err != nil {
		return CheckResult{Note: "fetch error: " + err.Error()}, ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // first 2MB is plenty
	body := string(b)
	lower := strings.ToLower(body)

	// Bot-mitigation / queue fingerprints. Pokemon Center returns 403 + these.
	if resp.StatusCode == 403 || resp.StatusCode == 429 || resp.StatusCode == 503 {
		if looksLikeBotWall(lower) || resp.StatusCode == 403 {
			return CheckResult{Blocked: true, Status: resp.StatusCode, Note: detectWall(lower, resp.StatusCode)}, body
		}
	}
	if looksLikeBotWall(lower) {
		return CheckResult{Blocked: true, Status: resp.StatusCode, Note: detectWall(lower, resp.StatusCode)}, body
	}
	if resp.StatusCode != 200 {
		return CheckResult{Status: resp.StatusCode, Note: fmt.Sprintf("unexpected http %d", resp.StatusCode)}, body
	}

	inStock, note := classifyStock(lower)
	return CheckResult{InStock: inStock, Status: resp.StatusCode, Note: note}, body
}

func looksLikeBotWall(lower string) bool {
	for _, s := range []string{
		"queue-it", "queueit", "please verify you are a human", "access denied",
		"akamai", "incapsula", "_incapsula_", "captcha", "cloudflare", "request unsuccessful",
		"are you a robot", "perimeterx", "kasada",
	} {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

func detectWall(lower string, status int) string {
	switch {
	case strings.Contains(lower, "queue-it") || strings.Contains(lower, "queueit"):
		return "Queue-it virtual queue active — join the queue in your real browser"
	case strings.Contains(lower, "captcha"):
		return "CAPTCHA/bot wall (Akamai/Incapsula)"
	case strings.Contains(lower, "access denied") || strings.Contains(lower, "akamai") || strings.Contains(lower, "incapsula"):
		return "Akamai/Incapsula 403 — need real browser session/proxy"
	default:
		return fmt.Sprintf("bot mitigation http=%d", status)
	}
}

// classifyStock uses conservative heuristics on Pokemon Center (Salesforce
// Commerce Cloud) markup. Returns inStock + evidence snippet.
func classifyStock(html string) (bool, string) {
	lower := strings.ToLower(html)
	hasAddToCart := strings.Contains(lower, "add to cart")
	hasAddToBag := strings.Contains(lower, "add to bag")
	hasOut := strings.Contains(lower, "out of stock")
	hasSoldOut := strings.Contains(lower, "sold out")
	hasNotify := strings.Contains(lower, "notify me when available") || strings.Contains(lower, "notify me")
	hasUnavailable := strings.Contains(lower, "this item is unavailable") || strings.Contains(lower, "currently unavailable")

	// Disabled add-to-cart button is the classic OOS state: the text is
	// present but the button is disabled / aria-disabled.
	disabled := strings.Contains(lower, "add to cart") &&
		(strings.Contains(lower, "disabled") || strings.Contains(lower, "aria-disabled=\"true\"") || strings.Contains(lower, "btn-disabled"))

	switch {
	case hasOut || hasSoldOut || hasUnavailable:
		return false, "oos keyword"
	case hasAddToCart || hasAddToBag:
		if disabled || hasNotify {
			return false, "add-to-cart present but disabled/notify-me"
		}
		return true, "add-to-cart enabled"
	default:
		return false, "no buy signals found"
	}
}

func alert(p Product, cfg Config, evidence string) {
	msg := fmt.Sprintf("IN STOCK: %s\n%s\n(%s)", p.Name, p.URL, evidence)
	fmt.Printf("\a>>> %s\n", msg) // terminal bell

	if cfg.BeepOnHit {
		beep()
	}
	if cfg.DiscordWebhook != "" {
		if err := sendDiscord(cfg.DiscordWebhook, msg); err != nil {
			log.Printf("discord alert failed: %v", err)
		} else {
			log.Printf("discord alert sent for %s", p.Name)
		}
	} else {
		log.Printf("no discord_webhook set — add one to config.json to get phone alerts")
	}
	if cfg.AutoOpenBrowser {
		openBrowser(p.URL)
	}
}

func sendDiscord(webhook, msg string) error {
	payload, _ := json.Marshal(map[string]string{"content": "🚨 " + msg})
	req, err := http.NewRequest(http.MethodPost, webhook, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("webhook http=%d %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("auto-open failed: %v", err)
	}
}

func beep() {
	// Three terminal bells; macOS `afplay` ping as best-effort.
	for i := 0; i < 3; i++ {
		fmt.Print("\a")
		time.Sleep(150 * time.Millisecond)
	}
	if runtime.GOOS == "darwin" {
		_ = exec.Command("afplay", "/System/Library/Sounds/Glass.aiff").Start()
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
