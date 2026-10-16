package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// DropJob fires one checkout attempt at an exact time, retries a few times
// on miss, then dies. Created from the dashboard for hype drops.
type DropJob struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	URL             string    `json:"url"`
	FireAt          time.Time `json:"fire_at"`
	MaxAttempts     int       `json:"max_attempts"`
	RetryIntervalMs int       `json:"retry_interval_ms"`
	PlaceOrder      bool      `json:"place_order"`
	Headed          bool      `json:"headed"`
	State           string    `json:"state"` // scheduled | running | done | dead | cancelled
	Attempts        int       `json:"attempts"`
	Note            string    `json:"note"`
	CheckoutRunID   string    `json:"checkout_run_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

var drops = newDropStore()

// onDropFound fires the browser checkout. Variable (not const) so tests can
// stub it without launching a real browser.
var onDropFound = func(name, url string, p Profile, headed, placeOrder bool) string {
	return startCheckoutRun(name, url, p, headed, placeOrder).ID
}

// scheduleDrop validates + registers a job and arms its timer.
func scheduleDrop(in DropJob, profilePath string) (*DropJob, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, fmt.Errorf("name required")
	}
	if err := validateProductURL(in.URL); err != nil {
		return nil, err
	}
	if in.FireAt.Before(time.Now().Add(-time.Minute)) {
		return nil, fmt.Errorf("fire_at is in the past")
	}
	if in.MaxAttempts <= 0 {
		in.MaxAttempts = 5
	}
	if in.MaxAttempts > 20 {
		return nil, fmt.Errorf("max_attempts capped at 20")
	}
	if in.RetryIntervalMs <= 0 {
		in.RetryIntervalMs = 2000
	}
	if in.RetryIntervalMs < 500 {
		return nil, fmt.Errorf("retry_interval_ms minimum 500 (rate-limit safety)")
	}
	job := &DropJob{
		ID:              dropJobID(time.Now()),
		Name:            in.Name,
		URL:             in.URL,
		FireAt:          in.FireAt,
		MaxAttempts:     in.MaxAttempts,
		RetryIntervalMs: in.RetryIntervalMs,
		PlaceOrder:      in.PlaceOrder,
		Headed:          in.Headed,
		State:           "scheduled",
		CreatedAt:       time.Now(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	drops.add(job, cancel)
	go runDropJob(ctx, job, profilePath)
	return job.clone(), nil
}

// runDropJob waits for fire time, then tries up to MaxAttempts fast stock
// checks spaced RetryIntervalMs apart. First in-stock signal hands off to
// ONE browser checkout run; all misses exhaust -> job dies (dead).
func runDropJob(ctx context.Context, job *DropJob, profilePath string) {
	drops.setState(job.ID, "scheduled", fmt.Sprintf("fires at %s", job.FireAt.Format("15:04:05")))
	timer := time.NewTimer(time.Until(job.FireAt))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		drops.setState(job.ID, "cancelled", "cancelled before firing")
		return
	case <-timer.C:
	}
	drops.setState(job.ID, "running", "fired — hunting Add to Cart")
	client := newClient(10)
	for i := 1; i <= job.MaxAttempts; i++ {
		select {
		case <-ctx.Done():
			drops.setState(job.ID, "cancelled", fmt.Sprintf("cancelled after %d attempt(s)", i-1))
			return
		default:
		}
		drops.setAttempts(job.ID, i)

		res, _ := fetchProduct(client, job.URL)
		log.Printf("drop %s attempt %d/%d: blocked=%v inStock=%v (%s)",
			job.ID, i, job.MaxAttempts, res.Blocked, res.InStock, res.Note)
		if res.InStock {
			p, _ := loadProfile(profilePath)
			runID := onDropFound(job.Name, job.URL, p, job.Headed, job.PlaceOrder)
			drops.setCheckoutRun(job.ID, runID)
			drops.setState(job.ID, "done", fmt.Sprintf("in-stock on attempt %d — browser checkout %s", i, runID))
			return
		}
		if i < job.MaxAttempts {
			select {
			case <-ctx.Done():
				drops.setState(job.ID, "cancelled", fmt.Sprintf("cancelled after %d attempt(s)", i))
				return
			case <-time.After(time.Duration(job.RetryIntervalMs) * time.Millisecond):
			}
		}
	}
	// Missed every attempt: kill the job. Nothing lingers.
	drops.setState(job.ID, "dead", fmt.Sprintf("no stock after %d attempts — job killed", job.MaxAttempts))
	drops.forget(job.ID)
}

func handleDrops(profilePath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, drops.List())
		case http.MethodPost:
			var in struct {
				Name            string `json:"name"`
				URL             string `json:"url"`
				FireAt          string `json:"fire_at"` // RFC3339
				MaxAttempts     int    `json:"max_attempts"`
				RetryIntervalMs int    `json:"retry_interval_ms"`
				PlaceOrder      bool   `json:"place_order"`
				Headed          *bool  `json:"headed"` // nil = default true (safer vs bot wall)
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in); err != nil {
				writeError(w, http.StatusBadRequest, "bad json")
				return
			}
			fireAt, err := parseFireAt(in.FireAt)
			if err != nil {
				writeError(w, http.StatusBadRequest, "fire_at must be RFC3339 (e.g. 2026-10-15T11:00:00-07:00)")
				return
			}
			job, err := scheduleDrop(DropJob{
				Name: in.Name, URL: in.URL, FireAt: fireAt,
				MaxAttempts: in.MaxAttempts, RetryIntervalMs: in.RetryIntervalMs,
				PlaceOrder: in.PlaceOrder, Headed: headedOrDefault(in.Headed),
			}, profilePath)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, job)
		default:
			writeError(w, http.StatusMethodNotAllowed, "use GET or POST")
		}
	}
}

func handleDropByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/drops/")
	id = strings.SplitN(id, "/", 2)[0]
	job, ok := drops.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "drop not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, job)
	case http.MethodDelete:
		if !drops.cancelJob(id) && (job.State == "scheduled" || job.State == "running") {
			drops.setState(id, "cancelled", "cancelled")
		}
		writeJSON(w, drops.mustGet(id))
	default:
		writeError(w, http.StatusMethodNotAllowed, "use GET or DELETE")
	}
}

func headedOrDefault(b *bool) bool {
	if b == nil {
		return true
	}
	return *b
}

// parseFireAt accepts RFC3339 plus common variants (e.g. macOS date emits
// -0700 without the colon).
func parseFireAt(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02T15:04:05-0700", s)
}
