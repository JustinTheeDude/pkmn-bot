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
func startUIServer(addr, profilePath string, store *configStore) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("bad --ui address %q: %w", addr, err)
	}
	if host != "" && host != "127.0.0.1" && host != "localhost" {
		return fmt.Errorf("refusing to bind %q: UI must stay on 127.0.0.1 for card-data safety", addr)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/settings", makeSettingsHandler(store))
	mux.HandleFunc("/api/products", makeProductsHandler(store))
	mux.HandleFunc("/api/products/", makeProductByIndexHandler(store))
	mux.HandleFunc("/api/profile", makeProfileHandler(profilePath))
	mux.HandleFunc("/api/checkout/", makeCheckoutHandler(store, profilePath))
	mux.HandleFunc("/api/checkout/runs", handleCheckoutRuns)
	mux.HandleFunc("/api/checkout/runs/", handleCheckoutRunByID)
	mux.HandleFunc("/api/drops", handleDrops(profilePath))
	mux.HandleFunc("/api/drops/", handleDropByID)
	mux.HandleFunc("/api/discover", makeDiscoverHandler(store))
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

func makeSettingsHandler(store *configStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, store.Settings())
		case http.MethodPut:
			var in settingsDTO
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
				writeError(w, http.StatusBadRequest, "bad json: "+err.Error())
				return
			}
			saved, err := store.UpdateSettings(in)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			store.updateStatus("settings saved")
			writeJSON(w, saved)
		default:
			writeError(w, http.StatusMethodNotAllowed, "use GET or PUT")
		}
	}
}

func makeProductsHandler(store *configStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, store.Products())
		case http.MethodPost:
			var item Product
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&item); err != nil {
				writeError(w, http.StatusBadRequest, "bad json")
				return
			}
			if strings.TrimSpace(item.Name) == "" {
				writeError(w, http.StatusBadRequest, "name required")
				return
			}
			if err := store.AddProduct(item); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			store.updateStatus("product added — restart monitor to apply")
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, item)
		default:
			writeError(w, http.StatusMethodNotAllowed, "use GET or POST")
		}
	}
}

// makeProductByIndexHandler serves DELETE /api/products/{index}.
func makeProductByIndexHandler(store *configStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeError(w, http.StatusMethodNotAllowed, "use DELETE")
			return
		}
		idx := pathIndex(r.URL.Path, "/api/products/")
		if err := store.RemoveProduct(idx); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		store.updateStatus("product removed — restart monitor to apply")
		writeJSON(w, map[string]string{"ok": "true"})
	}
}

// pathIndex extracts the leading integer from /prefix/{n}[/...].
func pathIndex(path, prefix string) int {
	rest := strings.TrimPrefix(path, prefix)
	rest = strings.SplitN(rest, "/", 2)[0]
	idx, err := strconv.Atoi(rest)
	if err != nil {
		return -1
	}
	return idx
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
func makeCheckoutHandler(store *configStore, profilePath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "use POST")
			return
		}
		products := store.Products()
		idx := pathIndex(r.URL.Path, "/api/checkout/")
		if idx < 0 || idx >= len(products) {
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
		run := startCheckoutRun(products[idx].Name, products[idx].URL, p, in.Headed, in.PlaceOrder)
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
