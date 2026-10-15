package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// statusSnapshot is served to the dashboard.
type statusSnapshot struct {
	Watching  int       `json:"watching"`
	Interval  int       `json:"interval_sec"`
	UpdatedAt time.Time `json:"updated_at"`
	Note      string    `json:"note"`
}

// settingsDTO is the REST representation of monitor settings
// (config minus the products list).
type settingsDTO struct {
	CheckIntervalSec int            `json:"check_interval_sec"`
	JitterMs         int            `json:"jitter_ms"`
	TimeoutSec       int            `json:"timeout_sec"`
	DiscordWebhook   string         `json:"discord_webhook"`
	AutoOpenBrowser  bool           `json:"auto_open_browser"`
	BeepOnHit        bool           `json:"beep_on_hit"`
	Discover         DiscoverConfig `json:"discover"`
}

var uiStatus = struct {
	sync.RWMutex
	snap statusSnapshot
}{}

func setUIStatus(watching, interval int, note string) {
	uiStatus.Lock()
	uiStatus.snap = statusSnapshot{Watching: watching, Interval: interval, UpdatedAt: time.Now(), Note: note}
	uiStatus.Unlock()
}

// startUIServer binds to 127.0.0.1 ONLY (never 0.0.0.0) so the form with
// address/card data is reachable only from this machine.
//
// REST API:
//
//	GET    /api/status            monitor snapshot
//	GET    /api/settings          monitor settings
//	PUT    /api/settings          update settings
//	GET    /api/products          list products
//	POST   /api/products          {name, url} -> create
//	DELETE /api/products/{index}  delete by index
//	GET    /api/profile           masked profile (never full PAN/CVV)
//	PUT    /api/profile           update profile (blank card fields = keep)
//
// Static React build (web/dist) is served at / when present.
func startUIServer(addr, cfgPath, profilePath string, cfg *Config) error {
	host, _, _ := net.SplitHostPort(addr)
	if host != "" && host != "127.0.0.1" && host != "localhost" {
		return fmt.Errorf("refusing to bind %q: UI must stay on 127.0.0.1 for card-data safety", addr)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/settings", makeSettingsHandler(cfgPath, cfg))
	mux.HandleFunc("/api/config", makeConfigHandler(cfgPath, cfg)) // legacy compat
	mux.HandleFunc("/api/products", makeProductsHandler(cfgPath, cfg))
	mux.HandleFunc("/api/products/", makeProductByIndexHandler(cfgPath, cfg))
	mux.HandleFunc("/api/profile", makeProfileHandler(profilePath))
	mux.HandleFunc("/api/checkout/", makeCheckoutHandler(cfg, profilePath))
	mux.HandleFunc("/api/checkout/runs", handleCheckoutRuns)
	mux.HandleFunc("/api/checkout/runs/", handleCheckoutRunByID)
	mux.HandleFunc("/api/drops", handleDrops(profilePath))
	mux.HandleFunc("/api/drops/", handleDropByID)
	mux.HandleFunc("/api/discover", handleDiscover(cfg, cfgPath))
	mux.HandleFunc("/api/discovered", handleDiscovered)
	mux.Handle("/", staticHandler())

	log.Printf("API: http://%s/api/status (localhost only — never expose publicly)", addr)
	return http.ListenAndServe(addr, mux)
}

// staticHandler serves the React build from web/dist next to the binary,
// falling back to a hint JSON when the frontend hasn't been built yet.
func staticHandler() http.Handler {
	dist := filepath.Join(exeDir(), "web", "dist")
	if _, err := os.Stat(filepath.Join(dist, "index.html")); err != nil {
		// Dev fallback: repo-relative path when running via `go run .`.
		if _, err2 := os.Stat("web/dist/index.html"); err2 == nil {
			dist = "web/dist"
		} else {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, map[string]string{
					"ok":   "true",
					"hint": "API is up. Build the React UI: cd web && npm install && npm run build, then reload /",
				})
			})
		}
	}
	fs := http.FileServer(http.Dir(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		// SPA fallback: serve index.html for unknown non-file routes.
		p := filepath.Join(dist, filepath.Clean("/"+r.URL.Path))
		if info, err := os.Stat(p); err != nil || info.IsDir() {
			http.ServeFile(w, r, filepath.Join(dist, "index.html"))
			return
		}
		fs.ServeHTTP(w, r)
	})
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	uiStatus.RLock()
	defer uiStatus.RUnlock()
	writeJSON(w, uiStatus.snap)
}

func settingsFrom(cfg *Config) settingsDTO {
	return settingsDTO{
		CheckIntervalSec: cfg.CheckIntervalSec,
		JitterMs:         cfg.JitterMs,
		TimeoutSec:       cfg.TimeoutSec,
		DiscordWebhook:   cfg.DiscordWebhook,
		AutoOpenBrowser:  cfg.AutoOpenBrowser,
		BeepOnHit:        cfg.BeepOnHit,
		Discover:         cfg.Discover,
	}
}

func makeSettingsHandler(cfgPath string, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, settingsFrom(cfg))
		case http.MethodPut:
			var in settingsDTO
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
				writeError(w, http.StatusBadRequest, "bad json: "+err.Error())
				return
			}
			if in.CheckIntervalSec < 10 {
				writeError(w, http.StatusBadRequest, "check_interval_sec must be >= 10 (rate-limit safety)")
				return
			}
			if in.TimeoutSec < 5 {
				writeError(w, http.StatusBadRequest, "timeout_sec must be >= 5")
				return
			}
			cfg.CheckIntervalSec = in.CheckIntervalSec
			cfg.JitterMs = in.JitterMs
			cfg.TimeoutSec = in.TimeoutSec
			cfg.DiscordWebhook = in.DiscordWebhook
			cfg.AutoOpenBrowser = in.AutoOpenBrowser
			cfg.BeepOnHit = in.BeepOnHit
			cfg.Discover = in.Discover
			normalizeDiscover(&cfg.Discover)
			if err := saveConfig(cfgPath, cfg); err != nil {
				writeError(w, http.StatusInternalServerError, "save: "+err.Error())
				return
			}
			setUIStatus(len(cfg.Products), cfg.CheckIntervalSec, "settings saved")
			writeJSON(w, settingsFrom(cfg))
		default:
			writeError(w, http.StatusMethodNotAllowed, "use GET or PUT")
		}
	}
}

func makeConfigHandler(cfgPath string, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, cfg)
		case http.MethodPost:
			var in Config
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
				writeError(w, http.StatusBadRequest, "bad json: "+err.Error())
				return
			}
			if in.CheckIntervalSec < 10 {
				writeError(w, http.StatusBadRequest, "check_interval_sec must be >= 10 (rate-limit safety)")
				return
			}
			cfg.CheckIntervalSec = in.CheckIntervalSec
			cfg.JitterMs = in.JitterMs
			cfg.TimeoutSec = in.TimeoutSec
			cfg.DiscordWebhook = in.DiscordWebhook
			cfg.AutoOpenBrowser = in.AutoOpenBrowser
			cfg.BeepOnHit = in.BeepOnHit
			if err := saveConfig(cfgPath, cfg); err != nil {
				writeError(w, http.StatusInternalServerError, "save: "+err.Error())
				return
			}
			setUIStatus(len(cfg.Products), cfg.CheckIntervalSec, "settings saved")
			writeJSON(w, map[string]string{"ok": "true"})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func makeProductsHandler(cfgPath string, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if cfg.Products == nil {
				writeJSON(w, []Product{})
				return
			}
			writeJSON(w, cfg.Products)
		case http.MethodPost:
			// New REST style: {name, url, ...} creates a product.
			// Legacy style: {action: add|remove, index, item} still works.
			var raw map[string]json.RawMessage
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&raw); err != nil {
				writeError(w, http.StatusBadRequest, "bad json")
				return
			}
			if _, legacy := raw["action"]; legacy {
				handleLegacyProductAction(w, cfgPath, cfg, raw)
				return
			}
			var item Product
			data, _ := json.Marshal(raw)
			if err := json.Unmarshal(data, &item); err != nil {
				writeError(w, http.StatusBadRequest, "bad product json")
				return
			}
			if item.Name == "" || item.URL == "" {
				writeError(w, http.StatusBadRequest, "name + url required")
				return
			}
			if !strings.HasPrefix(item.URL, "https://") {
				writeError(w, http.StatusBadRequest, "url must start with https://")
				return
			}
			item.NotifyOnly = true
			cfg.Products = append(cfg.Products, item)
			if err := saveConfig(cfgPath, cfg); err != nil {
				writeError(w, http.StatusInternalServerError, "save: "+err.Error())
				return
			}
			setUIStatus(len(cfg.Products), cfg.CheckIntervalSec, "product added — restart monitor to apply")
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, item)
		default:
			writeError(w, http.StatusMethodNotAllowed, "use GET or POST")
		}
	}
}

func handleLegacyProductAction(w http.ResponseWriter, cfgPath string, cfg *Config, raw map[string]json.RawMessage) {
	var in struct {
		Action string  `json:"action"`
		Index  int     `json:"index"`
		Item   Product `json:"item"`
	}
	data, _ := json.Marshal(raw)
	if err := json.Unmarshal(data, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad json")
		return
	}
	switch in.Action {
	case "add":
		if in.Item.URL == "" || in.Item.Name == "" {
			writeError(w, http.StatusBadRequest, "name + url required")
			return
		}
		cfg.Products = append(cfg.Products, in.Item)
	case "remove":
		if in.Index < 0 || in.Index >= len(cfg.Products) {
			writeError(w, http.StatusBadRequest, "bad index")
			return
		}
		cfg.Products = append(cfg.Products[:in.Index], cfg.Products[in.Index+1:]...)
	default:
		writeError(w, http.StatusBadRequest, "action must be add|remove")
		return
	}
	if err := saveConfig(cfgPath, cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "save: "+err.Error())
		return
	}
	setUIStatus(len(cfg.Products), cfg.CheckIntervalSec, "products updated — restart monitor to apply")
	writeJSON(w, cfg.Products)
}

// makeProductByIndexHandler serves DELETE /api/products/{index}.
func makeProductByIndexHandler(cfgPath string, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeError(w, http.StatusMethodNotAllowed, "use DELETE")
			return
		}
		idxStr := strings.TrimPrefix(r.URL.Path, "/api/products/")
		idxStr = strings.SplitN(idxStr, "/", 2)[0]
		idx, err := strconv.Atoi(idxStr)
		if err != nil || idx < 0 || idx >= len(cfg.Products) {
			writeError(w, http.StatusNotFound, "product index not found")
			return
		}
		cfg.Products = append(cfg.Products[:idx], cfg.Products[idx+1:]...)
		if err := saveConfig(cfgPath, cfg); err != nil {
			writeError(w, http.StatusInternalServerError, "save: "+err.Error())
			return
		}
		setUIStatus(len(cfg.Products), cfg.CheckIntervalSec, "product removed — restart monitor to apply")
		writeJSON(w, map[string]string{"ok": "true"})
	}
}

func makeProfileHandler(profilePath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			p, _ := loadProfile(profilePath)
			writeJSON(w, maskedProfile(p))
		case http.MethodPut, http.MethodPost:
			var in Profile
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&in); err != nil {
				writeError(w, http.StatusBadRequest, "bad json")
				return
			}
			// Empty card fields = keep existing (so masked GET can't wipe it).
			existing, _ := loadProfile(profilePath)
			if in.CardNumber == "" {
				in.CardNumber = existing.CardNumber
			}
			if in.CVV == "" {
				in.CVV = existing.CVV
			}
			if in.Country == "" {
				in.Country = "US"
			}
			if err := saveProfile(profilePath, in); err != nil {
				writeError(w, http.StatusBadRequest, "save: "+err.Error())
				return
			}
			// Do NOT log profile contents.
			log.Printf("API: profile saved to %s (0600)", profilePath)
			writeJSON(w, maskedProfile(in))
		default:
			writeError(w, http.StatusMethodNotAllowed, "use GET or PUT")
		}
	}
}

// POST /api/checkout/{index} starts an automated browser checkout run.
// Body: {"place_order": false, "headed": true}. Defaults to dry-run:
// the browser fills everything and stops before spending money.
func makeCheckoutHandler(cfg *Config, profilePath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "use POST")
			return
		}
		idxStr := strings.TrimPrefix(r.URL.Path, "/api/checkout/")
		idxStr = strings.SplitN(idxStr, "/", 2)[0]
		idx, err := strconv.Atoi(idxStr)
		if err != nil || idx < 0 || idx >= len(cfg.Products) {
			writeError(w, http.StatusNotFound, "product index not found")
			return
		}
		var in struct {
			PlaceOrder bool `json:"place_order"`
			Headed     bool `json:"headed"`
		}
		in.Headed = true
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in)
		p, _ := loadProfile(profilePath)
		run := startCheckoutRun(cfg.Products[idx].Name, cfg.Products[idx].URL, p, in.Headed, in.PlaceOrder)
		w.WriteHeader(http.StatusAccepted)
		writeJSON(w, run)
	}
}

func handleCheckoutRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	checkoutRuns.Lock()
	defer checkoutRuns.Unlock()
	list := make([]*checkoutRun, 0, len(checkoutRuns.runs))
	for _, v := range checkoutRuns.runs {
		list = append(list, v)
	}
	writeJSON(w, list)
}

func handleCheckoutRunByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/checkout/runs/")
	checkoutRuns.Lock()
	defer checkoutRuns.Unlock()
	run, ok := checkoutRuns.runs[id]
	if !ok {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	writeJSON(w, run)
}
