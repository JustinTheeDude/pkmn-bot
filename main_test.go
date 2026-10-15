package main

import "testing"

func TestClassifyStock(t *testing.T) {
	cases := []struct {
		name    string
		html    string
		inStock bool
	}{
		{"in stock", `<button>Add to Cart</button>`, true},
		{"oos keyword", `<button>Add to Cart</button><div>Out of Stock</div>`, false},
		{"sold out", `<div>Sold Out</div>`, false},
		{"disabled button", `<button disabled>Add to Cart</button><div>Notify me when available</div>`, false},
		{"no signals", `<div>hello</div>`, false},
	}
	for _, c := range cases {
		got, _ := classifyStock(c.html)
		if got != c.inStock {
			t.Errorf("%s: got %v want %v", c.name, got, c.inStock)
		}
	}
}

func TestLooksLikeBotWall(t *testing.T) {
	if !looksLikeBotWall("queue-it please wait captcha akamai access denied") {
		t.Error("should detect bot wall")
	}
	if looksLikeBotWall("<button>add to cart</button> in stock") {
		t.Error("false positive on clean page")
	}
}
