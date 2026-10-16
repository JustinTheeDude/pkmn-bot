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
	"sync"
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
	sync.RWMutex
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
	return titleWords(slug)
}

// titleWords upper-cases the first letter of each space-separated word.
// Replaces the deprecated strings.Title.
func titleWords(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	startOfWord := true
	for _, r := range s {
		if r == ' ' {
			startOfWord = true
			b.WriteRune(r)
			continue
		}
		if startOfWord {
			b.WriteString(strings.ToUpper(string(r)))
			startOfWord = false
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
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
func autoBuyMatches(matches []string, d DiscoverConfig, profilePath string) []string {
	if d.AutoBuy != "dry-run" && d.AutoBuy != "live" {
		return nil
	}
	live := d.AutoBuy == "live"
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

func discoverLoop(store *configStore, profilePath string) {
	known := loadKnownURLs(knownURLsFile)
	log.Printf("discover: polling sitemap (known=%d)", len(known))
	ticker := time.NewTicker(discoverTickInterval(store))
	defer ticker.Stop()
	for range ticker.C {
		d := store.Discover()
		if !d.Enabled {
			continue
		}
		matches, all, seeded, err := discoverOnce(newClient(30), sitemapProducts, known, d.Keywords)
		if err != nil {
			setDiscoverState(time.Now(), len(known), 0, nil)
			log.Printf("discover: feed fetch failed (wall?): %v", err)
			continue
		}
		saveKnownURLs(knownURLsFile, all)
		setDiscoverState(time.Now(), len(all), 0, nil)
		if seeded {
			log.Printf("discover: seeded %d known URLs", len(all))
			continue
		}
		if len(matches) == 0 {
			continue
		}
		// Only genuinely-new URLs (not already on the watchlist) are worth a
		// notification or an auto-buy — a URL we already watch is handled by
		// the regular monitor and must never be double-bought here.
		addMatches, err := store.AddDiscoveredBatch(matches)
		if err != nil {
			log.Printf("discover: save config: %v", err)
		}
		if len(addMatches) == 0 {
			continue
		}
		for _, m := range addMatches {
			log.Printf("discover: NEW URL %s", m)
		}
		runs := autoBuyMatches(addMatches, d, profilePath)
		setDiscoverState(time.Now(), len(all), len(addMatches), addMatches)
		note := fmt.Sprintf("%d new URL(s) discovered — restart monitor to watch them", len(addMatches))
		if len(runs) > 0 {
			note = fmt.Sprintf("%d new URL(s) discovered, %d in-stock -> checkout %s", len(addMatches), len(runs), strings.Join(runs, ","))
		}
		store.updateStatus(note)
		discordMsg := fmt.Sprintf("🆕 NEW PRODUCT URLS (%d):\n%s\nRestart monitor to watch them.",
			len(addMatches), strings.Join(addMatches, "\n"))
		if len(runs) > 0 {
			discordMsg = fmt.Sprintf("🆕 NEW PRODUCT URLS (%d), %d IN-STOCK:\n%s\nAuto-buy checkout(s): %s",
				len(addMatches), len(runs), strings.Join(addMatches, "\n"), strings.Join(runs, ", "))
		}
		_ = sendDiscord(store.Webhook(), discordMsg)
	}
}

func discoverTickInterval(store *configStore) time.Duration {
	mins := store.Discover().IntervalMin
	if mins <= 0 {
		mins = 5
	}
	return time.Duration(mins) * time.Minute
}

// setDiscoverState publishes discovery progress for the dashboard.
func setDiscoverState(lastCheck time.Time, knownCount, lastNew int, matched []string) {
	discoverStatus.Lock()
	discoverStatus.state.LastCheck = lastCheck
	discoverStatus.state.KnownCount = knownCount
	if lastNew > 0 {
		discoverStatus.state.LastNew = lastNew
		discoverStatus.state.LastMatched = matched
	}
	discoverStatus.Unlock()
}

func discoverSnapshot() discoverState {
	discoverStatus.RLock()
	defer discoverStatus.RUnlock()
	s := discoverStatus.state
	s.LastMatched = append([]string(nil), discoverStatus.state.LastMatched...)
	return s
}

func makeDiscoverHandler(store *configStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			snap := discoverSnapshot()
			writeJSON(w, map[string]any{
				"config":      store.Discover(),
				"last_check":  snap.LastCheck,
				"known_count": snap.KnownCount,
				"last_new":    snap.LastNew,
			})
		case http.MethodPut:
			var in DiscoverConfig
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in); err != nil {
				writeError(w, http.StatusBadRequest, "bad json")
				return
			}
			saved, err := store.UpdateDiscover(in)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "save: "+err.Error())
				return
			}
			writeJSON(w, saved)
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
	matched := discoverSnapshot().LastMatched
	if matched == nil {
		matched = []string{}
	}
	writeJSON(w, matched)
}
