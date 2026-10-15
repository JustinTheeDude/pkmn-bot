package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestScheduledDropHitsAndHandsOff(t *testing.T) {
	srv := fakeStore() // /product/1 has an enabled Add to Cart button
	defer srv.Close()

	orig := onDropFound
	calls := 0
	onDropFound = func(name, url string, p Profile, headed, placeOrder bool) string {
		calls++
		return "co-test-1"
	}
	defer func() { onDropFound = orig }()

	job, err := scheduleDrop(DropJob{
		Name:            "Test Box",
		URL:             srv.URL + "/product/1",
		FireAt:          time.Now().Add(200 * time.Millisecond),
		MaxAttempts:     5,
		RetryIntervalMs: 500,
	}, t.TempDir()+"/profile.json")
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		dropJobs.Lock()
		state := dropJobs.jobs[job.ID].State
		dropJobs.Unlock()
		if state == "done" {
			break
		}
		if state == "dead" {
			t.Fatalf("job died, expected done: %+v", job)
		}
		time.Sleep(100 * time.Millisecond)
	}
	dropJobs.Lock()
	defer dropJobs.Unlock()
	got := dropJobs.jobs[job.ID]
	if got.State != "done" {
		t.Fatalf("state=%s note=%s", got.State, got.Note)
	}
	if calls != 1 || got.CheckoutRunID != "co-test-1" {
		t.Fatalf("expected one browser handoff, calls=%d job=%+v", calls, got)
	}
}

func TestScheduledDropDiesAfterMisses(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/product/oos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><div>Out of Stock</div><button disabled>Notify me when available</button></body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	orig := onDropFound
	calls := 0
	onDropFound = func(name, url string, p Profile, headed, placeOrder bool) string {
		calls++
		return "co-nope"
	}
	defer func() { onDropFound = orig }()

	job, err := scheduleDrop(DropJob{
		Name:            "Gone Box",
		URL:             srv.URL + "/product/oos",
		FireAt:          time.Now().Add(100 * time.Millisecond),
		MaxAttempts:     2,
		RetryIntervalMs: 500,
	}, t.TempDir()+"/profile.json")
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		dropJobs.Lock()
		state := dropJobs.jobs[job.ID].State
		dropJobs.Unlock()
		if state == "dead" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	dropJobs.Lock()
	defer dropJobs.Unlock()
	got := dropJobs.jobs[job.ID]
	if got.State != "dead" {
		t.Fatalf("state=%s note=%s", got.State, got.Note)
	}
	if got.Attempts != 2 {
		t.Fatalf("attempts=%d want 2", got.Attempts)
	}
	if calls != 0 {
		t.Fatal("browser must never launch on a full miss")
	}
}
