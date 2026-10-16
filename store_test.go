package main

import (
	"fmt"
	"sync"
	"testing"
)

func newTestStore(t *testing.T) *configStore {
	t.Helper()
	store, err := newConfigStore(t.TempDir() + "/config.json")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

// TestConfigStoreConcurrentAccess is the regression test for the data races
// that existed when every component shared one *Config. Run with -race.
func TestConfigStoreConcurrentAccess(t *testing.T) {
	store := newTestStore(t)

	var wg sync.WaitGroup
	wg.Add(3)
	go func() { // settings writer
		defer wg.Done()
		for i := 0; i < 100; i++ {
			if _, err := store.UpdateSettings(settingsDTO{
				CheckIntervalSec: 20, TimeoutSec: 15,
				DiscordWebhook: fmt.Sprintf("hook-%d", i),
				Discover:       store.Discover(),
			}); err != nil {
				t.Errorf("UpdateSettings: %v", err)
				return
			}
		}
	}()
	go func() { // discovery writer
		defer wg.Done()
		for i := 0; i < 100; i++ {
			if _, err := store.AddDiscovered("x", fmt.Sprintf("https://x/%d", i)); err != nil {
				t.Errorf("AddDiscovered: %v", err)
				return
			}
		}
	}()
	go func() { // monitor readers
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = store.Snapshot()
			_ = store.Products()
			store.WatchParams()
			_ = store.Webhook()
		}
	}()
	wg.Wait()
}

func TestAddDiscoveredDeduplicates(t *testing.T) {
	store := newTestStore(t)
	added, err := store.AddDiscovered("one", "https://www.pokemoncenter.com/product/1/x")
	if err != nil || !added {
		t.Fatalf("first add: added=%v err=%v", added, err)
	}
	added, err = store.AddDiscovered("one again", "https://www.pokemoncenter.com/product/1/x")
	if err != nil {
		t.Fatalf("second add err: %v", err)
	}
	if added {
		t.Fatal("duplicate URL must not be added twice")
	}
	if got := store.ProductCount(); got != 1 {
		t.Fatalf("count=%d want 1", got)
	}
}

func TestAddProductRejectsBadURL(t *testing.T) {
	store := newTestStore(t)
	if err := store.AddProduct(Product{Name: "x", URL: "ftp://nope"}); err == nil {
		t.Fatal("expected https:// validation error")
	}
	if err := store.AddProduct(Product{Name: "x", URL: "https://ok.example/p"}); err != nil {
		t.Fatalf("valid https URL rejected: %v", err)
	}
}

func TestUpdateSettingsValidation(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.UpdateSettings(settingsDTO{CheckIntervalSec: 5, TimeoutSec: 15}); err == nil {
		t.Fatal("interval < 10 must be rejected")
	}
	if _, err := store.UpdateSettings(settingsDTO{CheckIntervalSec: 20, TimeoutSec: 1}); err == nil {
		t.Fatal("timeout < 5 must be rejected")
	}
	if _, err := store.UpdateSettings(settingsDTO{CheckIntervalSec: 20, TimeoutSec: 15, Discover: DiscoverConfig{AutoBuy: "bogus"}}); err != nil {
		t.Fatalf("valid settings rejected: %v", err)
	}
	if got := store.Discover().AutoBuy; got != "off" {
		t.Fatalf("bad auto_buy should normalise to off, got %q", got)
	}
}

func TestSnapshotIsIsolated(t *testing.T) {
	store := newTestStore(t)
	_ = store.AddProduct(Product{Name: "a", URL: "https://a.example/1"})
	snap := store.Snapshot()
	snap.Products[0].Name = "mutated"
	if store.Products()[0].Name != "a" {
		t.Fatal("Snapshot must return a copy callers cannot mutate")
	}
}

func TestTitleWords(t *testing.T) {
	got := titleWords("scarlet and violet 151 booster bundle")
	want := "Scarlet And Violet 151 Booster Bundle"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
