package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeSitemap(urls []string) *httptest.Server {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, u := range urls {
		b.WriteString("<url><loc>" + u + "</loc></url>")
	}
	b.WriteString(`</urlset>`)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(b.String()))
	}))
}

func TestDiscoverSeedsThenMatches(t *testing.T) {
	srv := fakeSitemap([]string{
		"https://www.pokemoncenter.com/product/aaa/some-plush",
		"https://www.pokemoncenter.com/product/bbb/elite-trainer-box",
	})
	defer srv.Close()

	known := map[string]bool{}
	matches, all, seeded, err := discoverOnce(newClient(10), srv.URL, known, []string{"elite-trainer-box"})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if !seeded || len(matches) != 0 || len(all) != 2 {
		t.Fatalf("seeded=%v matches=%v all=%d", seeded, matches, len(all))
	}

	// Feed gains a day-of drop URL.
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` +
			`<url><loc>https://www.pokemoncenter.com/product/aaa/some-plush</loc></url>` +
			`<url><loc>https://www.pokemoncenter.com/product/bbb/elite-trainer-box</loc></url>` +
			`<url><loc>https://www.pokemoncenter.com/product/ccc/mega-evolution-booster-bundle</loc></url>` +
			`</urlset>`))
	})
	matches, _, seeded, err = discoverOnce(newClient(10), srv.URL, known, []string{"booster"})
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if seeded {
		t.Fatal("second run must not seed")
	}
	if len(matches) != 1 || !strings.Contains(matches[0], "booster-bundle") {
		t.Fatalf("matches=%v", matches)
	}
}

func TestDiffSitemapKeywords(t *testing.T) {
	known := map[string]bool{"https://x/old": true}
	got := diffSitemap(known, []string{
		"https://x/old",
		"https://www.pokemoncenter.com/product/1/Prismatic-Evolutions-ETB",
		"https://www.pokemoncenter.com/product/2/pikachu-plush",
	}, []string{"etb"})
	if len(got) != 1 {
		t.Fatalf("got=%v", got)
	}
}

func TestNormalizeDiscover(t *testing.T) {
	d := DiscoverConfig{IntervalMin: 0, AutoBuy: "bogus"}
	normalizeDiscover(&d)
	if d.IntervalMin != 5 || d.AutoBuy != "off" {
		t.Fatalf("got %+v", d)
	}
	d = DiscoverConfig{IntervalMin: 3, AutoBuy: "live"}
	normalizeDiscover(&d)
	if d.IntervalMin != 3 || d.AutoBuy != "live" {
		t.Fatalf("valid modes must survive, got %+v", d)
	}
}

func TestAutoBuyMatches(t *testing.T) {
	inStock := fakeStore() // /product/1 has an enabled Add to Cart button
	defer inStock.Close()

	oosMux := http.NewServeMux()
	oosMux.HandleFunc("/product/oos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><div>Out of Stock</div></body></html>`))
	})
	oos := httptest.NewServer(oosMux)
	defer oos.Close()

	orig := onDiscoverFound
	var calls []struct {
		url        string
		headed     bool
		placeOrder bool
	}
	onDiscoverFound = func(name, url string, p Profile, headed, placeOrder bool) string {
		calls = append(calls, struct {
			url        string
			headed     bool
			placeOrder bool
		}{url, headed, placeOrder})
		return "co-discover-1"
	}
	defer func() { onDiscoverFound = orig }()

	cfg := DiscoverConfig{Enabled: true, IntervalMin: 5, AutoBuy: "dry-run"}

	// OOS URL: no checkout, no run IDs.
	if runs := autoBuyMatches([]string{oos.URL + "/product/oos"}, cfg, t.TempDir()+"/profile.json"); len(runs) != 0 {
		t.Fatalf("oos must not buy, runs=%v", runs)
	}
	if len(calls) != 0 {
		t.Fatalf("browser must never launch on OOS, calls=%d", len(calls))
	}

	// In-stock URL in dry-run mode: one headed checkout, never live.
	runs := autoBuyMatches([]string{inStock.URL + "/product/1"}, cfg, t.TempDir()+"/profile.json")
	if len(runs) != 1 || runs[0] != "co-discover-1" {
		t.Fatalf("runs=%v", runs)
	}
	if len(calls) != 1 || !calls[0].headed || calls[0].placeOrder {
		t.Fatalf("want one headed dry-run, calls=%+v", calls)
	}

	// Live mode passes placeOrder through.
	calls = nil
	cfg.AutoBuy = "live"
	autoBuyMatches([]string{inStock.URL + "/product/1"}, cfg, t.TempDir()+"/profile.json")
	if len(calls) != 1 || !calls[0].placeOrder {
		t.Fatalf("live must place order, calls=%+v", calls)
	}

	// Off (and unknown values) never buy.
	calls = nil
	cfg.AutoBuy = "off"
	if runs := autoBuyMatches([]string{inStock.URL + "/product/1"}, cfg, t.TempDir()+"/profile.json"); len(runs) != 0 || len(calls) != 0 {
		t.Fatalf("off must not buy, runs=%v calls=%d", runs, len(calls))
	}
}
