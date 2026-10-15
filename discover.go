package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// DiscoverConfig polls the SFCC product sitemap for day-of drop URLs.
type DiscoverConfig struct {
	Enabled     bool     `json:"enabled"`
	IntervalMin int      `json:"interval_min"`
	Keywords    []string `json:"keywords"` // case-insensitive slug substrings
	// AutoBuy fires a browser checkout for discovered URLs that are in-stock
	// on a fast re-check: "off" (default), "dry-run" (fills, never spends),
	// or "live" (actually places the order — spends money).
	AutoBuy string `json:"auto_buy"`
}

func defaultDiscover() DiscoverConfig {
	return DiscoverConfig{
		Enabled:     true,
		IntervalMin: 5,
		Keywords:    []string{"elite-trainer-box", "booster", "collection", "tin", "etb"},
		AutoBuy:     "off",
	}
}

// normalizeDiscover fills defaults and rejects unknown auto_buy modes so a
// typo can never silently enable spending.
func normalizeDiscover(d *DiscoverConfig) {
	if d.IntervalMin <= 0 {
		d.IntervalMin = 5
	}
	switch d.AutoBuy {
	case "dry-run", "live":
	default:
		d.AutoBuy = "off"
	}
}

// Sitemap endpoints (SFCC). products.xml is the full catalog feed.
const (
	sitemapProducts = "https://www.pokemoncenter.com/sitemaps/products.xml"
	knownURLsFile   = "sitemap_known.json"
)

type discoverState struct {
	LastCheck   time.Time `json:"last_check"`
	KnownCount  int       `json:"known_count"`
	LastNew     int       `json:"last_new"`
	LastMatched []string  `json:"last_matched"`
}

var discoverStatus = struct {
	state discoverState
}{}

// fetchSitemapLocs streams <loc> entries without loading the whole feed.
func fetchSitemapLocs(client *http.Client, url string) ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgents[0])
	req.Header.Set("Accept", "application/xml,text/xml,*/*")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("sitemap http=%d", resp.StatusCode)
	}
	var locs []string
	dec := xml.NewDecoder(io.LimitReader(resp.Body, 64<<20))
	for {
		tok, err := dec.Token()
		if err != nil {
			break // io.EOF ends the stream
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "loc" {
			continue
		}
		var loc string
		if err := dec.DecodeElement(&loc, &se); err != nil {
			continue
		}
		loc = strings.TrimSpace(loc)
		if loc != "" {
			locs = append(locs, loc)
		}
	}
	if len(locs) == 0 {
		return nil, fmt.Errorf("no urls parsed (blocked or empty feed?)")
	}
	return locs, nil
}

// diffSitemap returns catalog URLs never seen before that match keywords.
func diffSitemap(known map[string]bool, locs []string, keywords []string) []string {
	var matches []string
	for _, loc := range locs {
		if known[loc] {
			continue
		}
		slug := strings.ToLower(loc)
		for _, kw := range keywords {
			kw = strings.ToLower(strings.TrimSpace(kw))
			if kw != "" && strings.Contains(slug, kw) {
				matches = append(matches, loc)
				break
			}
		}
	}
	return matches
}

func loadKnownURLs(path string) map[string]bool {
	known := map[string]bool{}
	data, err := os.ReadFile(path)
	if err != nil {
		return known
	}
	var locs []string
	if err := json.Unmarshal(data, &locs); err != nil {
		return known
	}
	for _, l := range locs {
		known[l] = true
	}
	return known
}

func saveKnownURLs(path string, locs []string) {
	data, err := json.Marshal(locs)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0644)
}

// discoverOnce fetches the feed, learns unseen URLs, and returns keyword
// matches. First run only seeds the known set (no alert storm).
func discoverOnce(client *http.Client, feedURL string, known map[string]bool, keywords []string) (matches []string, all []string, seeded bool, err error) {
	all, err = fetchSitemapLocs(client, feedURL)
	if err != nil {
		return nil, nil, false, err
	}
	if len(known) == 0 {
		for _, l := range all {
			known[l] = true
		}
		return nil, all, true, nil
	}
	matches = diffSitemap(known, all, keywords)
	for _, l := range all {
		known[l] = true
	}
	return matches, all, false, nil
}

func slugToName(url string) string {
	slug := url
	if i := strings.LastIndex(slug, "/"); i >= 0 {
		slug = slug[i+1:]
	}
	slug = strings.ReplaceAll(slug, "-", " ")
	if len(slug) > 60 {
		slug = slug[:60] + "…"
	}
	return strings.Title(slug)
}

// onDiscoverFound hands an in-stock discovered URL to a browser checkout.
// Variable (not const) so tests can stub it without launching a browser.
var onDiscoverFound = func(name, url string, p Profile, headed, placeOrder bool) string {
	return startCheckoutRun(name, url, p, headed, placeOrder).ID
}

// autoBuyMatches fast stock-checks freshly discovered URLs and fires ONE
// browser checkout run per in-stock hit when AutoBuy is enabled ("dry-run"
// fills everything and stops before spending, "live" places the order).
// OOS/blocked URLs are skipped — they stay in the watchlist for the regular
// monitor. Returns the checkout run IDs started.
func autoBuyMatches(matches []string, cfg *Config, profilePath string) []string {
	if cfg.Discover.AutoBuy != "dry-run" && cfg.Discover.AutoBuy != "live" {
		return nil
	}
	live := cfg.Discover.AutoBuy == "live"
	p, _ := loadProfile(profilePath)
	client := newClient(10)
	var started []string
	for _, m := range matches {
		res, _ := fetchProduct(client, m)
		if !res.InStock {
			log.Printf("discover autobuy: skip %s (blocked=%v :: %s)", m, res.Blocked, res.Note)
			continue
		}
		name := slugToName(m)
		runID := onDiscoverFound(name, m, p, true, live)
		log.Printf("discover autobuy: IN-STOCK %s -> checkout %s (live=%v)", m, runID, live)
		started = append(started, runID)
	}
	return started
}

func discoverLoop(cfg *Config, cfgPath, profilePath string) {
	interval := cfg.Discover.IntervalMin
	if interval <= 0 {
		interval = 5
	}
	known := loadKnownURLs(knownURLsFile)
	log.Printf("discover: watching sitemap for %v every %dm (known=%d)", cfg.Discover.Keywords, interval, len(known))
	ticker := time.NewTicker(time.Duration(interval) * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if !cfg.Discover.Enabled {
			continue
		}
		matches, all, seeded, err := discoverOnce(newClient(30), sitemapProducts, known, cfg.Discover.Keywords)
		discoverStatus.state.LastCheck = time.Now()
		discoverStatus.state.KnownCount = len(known)
		if err != nil {
			log.Printf("discover: feed fetch failed (wall?): %v", err)
			continue
		}
		saveKnownURLs(knownURLsFile, all)
		if seeded {
			log.Printf("discover: seeded %d known URLs", len(all))
			continue
		}
		if len(matches) == 0 {
			continue
		}
		discoverStatus.state.LastNew = len(matches)
		discoverStatus.state.LastMatched = matches
		for _, m := range matches {
			log.Printf("discover: NEW URL %s", m)
			dup := false
			for _, p := range cfg.Products {
				if p.URL == m {
					dup = true
					break
				}
			}
			if !dup {
				cfg.Products = append(cfg.Products, Product{Name: slugToName(m) + " (discovered)", URL: m, NotifyOnly: true})
			}
		}
		if err := saveConfig(cfgPath, cfg); err != nil {
			log.Printf("discover: save config: %v", err)
		}
		runs := autoBuyMatches(matches, cfg, profilePath)
		note := fmt.Sprintf("%d new URL(s) discovered — restart monitor to watch them", len(matches))
		if len(runs) > 0 {
			note = fmt.Sprintf("%d new URL(s) discovered, %d in-stock -> checkout %s", len(matches), len(runs), strings.Join(runs, ","))
		}
		setUIStatus(len(cfg.Products), cfg.CheckIntervalSec, note)
		discordMsg := fmt.Sprintf("🆕 NEW PRODUCT URLS (%d):\n%s\nRestart monitor to watch them.",
			len(matches), strings.Join(matches, "\n"))
		if len(runs) > 0 {
			discordMsg = fmt.Sprintf("🆕 NEW PRODUCT URLS (%d), %d IN-STOCK:\n%s\nAuto-buy checkout(s): %s",
				len(matches), len(runs), strings.Join(matches, "\n"), strings.Join(runs, ", "))
		}
		_ = sendDiscord(cfg.DiscordWebhook, discordMsg)
	}
}

func handleDiscover(cfg *Config, cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, map[string]any{
				"config":      cfg.Discover,
				"last_check":  discoverStatus.state.LastCheck,
				"known_count": discoverStatus.state.KnownCount,
				"last_new":    discoverStatus.state.LastNew,
			})
		case http.MethodPut:
			var in DiscoverConfig
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in); err != nil {
				writeError(w, http.StatusBadRequest, "bad json")
				return
			}
			if in.IntervalMin <= 0 {
				in.IntervalMin = 5
			}
			normalizeDiscover(&in)
			cfg.Discover = in
			if err := saveConfig(cfgPath, cfg); err != nil {
				writeError(w, http.StatusInternalServerError, "save: "+err.Error())
				return
			}
			writeJSON(w, cfg.Discover)
		default:
			writeError(w, http.StatusMethodNotAllowed, "use GET or PUT")
		}
	}
}

func handleDiscovered(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	matched := discoverStatus.state.LastMatched
	if matched == nil {
		matched = []string{}
	}
	writeJSON(w, matched)
}
