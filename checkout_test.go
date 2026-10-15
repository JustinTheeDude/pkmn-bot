package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// fakeStore serves a minimal product -> cart -> checkout flow so the
// Playwright checkout logic is verified without touching a real shop.
func fakeStore() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/product/1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>
<button onclick="location='/cart'">Add to Cart</button>
</body></html>`))
	})
	mux.HandleFunc("/cart", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>
<a href="/checkout">Checkout</a>
</body></html>`))
	})
	mux.HandleFunc("/checkout", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><form>
<input name="email"><input name="firstName"><input name="lastName">
<input name="phone"><input name="address1"><input name="address2">
<input name="city"><input name="state"><input name="zip">
<input name="cardNumber"><input name="expiry"><input name="cvv"><input name="cardName">
<button>Place Order</button>
</form></body></html>`))
	})
	return httptest.NewServer(mux)
}

func TestRunCheckoutDryRun(t *testing.T) {
	if testing.Short() {
		t.Skip("browser test")
	}
	srv := fakeStore()
	defer srv.Close()

	res := RunCheckout(CheckoutOptions{
		ProductURL: srv.URL + "/product/1",
		Profile: Profile{
			FirstName: "Test", LastName: "User", Email: "t@example.com",
			Address1: "123 Main St", City: "Irvine", State: "CA", Zip: "92602",
			CardNumber: "4111111111111111", ExpMonth: "08", ExpYear: "2028", CVV: "123",
		},
		Headed:      false,
		PlaceOrder:  false,
		UserDataDir: t.TempDir(),
		ShotDir:     t.TempDir(),
		TimeoutSec:  30,
	})
	if res.Error != "" {
		t.Fatalf("checkout failed: %s\nsteps: %s", res.Error, strings.Join(res.Steps, " | "))
	}
	if !res.OK || !res.DryRunStopped {
		t.Fatalf("expected dry-run stop, got %+v", res)
	}
	if res.OrderPlaced {
		t.Fatal("dry run must never place an order")
	}
	if st, err := os.Stat(res.Screenshot); err != nil || st.Size() == 0 {
		t.Fatalf("expected review screenshot at %s (err=%v)", res.Screenshot, err)
	}
}
