package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mxschmitt/playwright-go"
)

// CheckoutOptions controls one automated checkout run.
type CheckoutOptions struct {
	ProductURL string
	Profile    Profile
	// Headed opens a visible window. Recommended for Queue-it/Akamai drops:
	// you pass the queue/CAPTCHA once manually, automation takes over after.
	Headed bool
	// UserDataDir persists cookies/login between runs. Defaults to
	// ./browser-profile (gitignored) so you stay logged in.
	UserDataDir string
	// DryRun stops before clicking Place Order and saves a screenshot for
	// review. Default true — set PlaceOrder to actually spend money.
	PlaceOrder bool
	TimeoutSec int
	ShotDir    string
}

// CheckoutResult summarises a run. Card data is never included.
type CheckoutResult struct {
	OK            bool     `json:"ok"`
	Steps         []string `json:"steps"`
	OrderPlaced   bool     `json:"order_placed"`
	DryRunStopped bool     `json:"dry_run_stopped"`
	Screenshot    string   `json:"screenshot,omitempty"`
	Error         string   `json:"error,omitempty"`
	DurationMs    int64    `json:"duration_ms"`
}

func (o *CheckoutOptions) withDefaults() {
	if o.UserDataDir == "" {
		o.UserDataDir = "browser-profile"
	}
	if o.TimeoutSec <= 0 {
		o.TimeoutSec = 60
	}
	if o.ShotDir == "" {
		o.ShotDir = "screenshots"
	}
}

// RunCheckout drives a real Chromium through product -> cart -> checkout.
// It fills shipping + payment from profile but NEVER logs card fields.
func RunCheckout(o CheckoutOptions) CheckoutResult {
	start := time.Now()
	o.withDefaults()
	res := CheckoutResult{Steps: []string{}}
	step := func(s string) {
		res.Steps = append(res.Steps, s)
		log.Printf("checkout: %s", s)
	}

	if o.ProductURL == "" {
		res.Error = "empty product URL"
		return finishCheckout(start, res)
	}
	if err := os.MkdirAll(o.ShotDir, 0755); err != nil {
		res.Error = "shot dir: " + err.Error()
		return finishCheckout(start, res)
	}

	if err := os.MkdirAll(o.UserDataDir, 0755); err != nil {
		res.Error = "profile dir: " + err.Error()
		return finishCheckout(start, res)
	}
	stateFile := filepath.Join(o.UserDataDir, "storage.json")

	pw, err := playwright.Run()
	if err != nil {
		res.Error = "start playwright: " + err.Error()
		return finishCheckout(start, res)
	}
	defer func() { _ = pw.Stop() }()

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(!o.Headed),
		Args: []string{
			"--disable-blink-features=AutomationControlled",
		},
	})
	if err != nil {
		res.Error = "launch browser: " + err.Error()
		return finishCheckout(start, res)
	}
	defer func() { _ = browser.Close() }()

	ctxOpts := playwright.BrowserNewContextOptions{
		Viewport: &playwright.Size{Width: 1280, Height: 900},
	}
	// Reuse saved login session when present (written at end of each run).
	if _, err := os.Stat(stateFile); err == nil {
		ctxOpts.StorageStatePath = playwright.String(stateFile)
		step("loaded saved login session")
	}
	ctx, err := browser.NewContext(ctxOpts)
	if err != nil {
		res.Error = "new context: " + err.Error()
		return finishCheckout(start, res)
	}
	// Persist cookies/login for the next run (best-effort, never card data —
	// card fields are only ever typed, never stored by us).
	defer func() {
		_, _ = ctx.StorageState(playwright.BrowserContextStorageStateOptions{Path: playwright.String(stateFile)})
		_ = ctx.Close()
	}()

	page, err := ctx.NewPage()
	if err != nil {
		res.Error = "new page: " + err.Error()
		return finishCheckout(start, res)
	}

	ms := float64(o.TimeoutSec) * 1000

	step("open product page")
	if _, err = page.Goto(o.ProductURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(ms),
	}); err != nil {
		res.Error = "goto product: " + err.Error()
		return finishCheckout(start, res)
	}

	// Queue-it / CAPTCHA: pause for a human instead of failing silently.
	if isQueuePage(page) {
		step("queue/bot-check page detected — solve it in the browser window (5 min max), automation resumes after")
		if err = waitForQueuePass(page, 5*time.Minute); err != nil {
			res.Error = err.Error()
			return finishCheckout(start, res)
		}
		step("queue passed")
	}

	step("click Add to Cart")
	if err = clickFirstVisible(page, addToCartCandidates(), ms); err != nil {
		shot(page, o.ShotDir, "no-add-to-cart")
		res.Error = "add to cart not found (sold out?): " + err.Error()
		return finishCheckout(start, res)
	}
	step("added to cart")

	step("open cart")
	cartURL := cartURLFor(o.ProductURL)
	if _, err = page.Goto(cartURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(ms),
	}); err != nil {
		res.Error = "goto cart: " + err.Error()
		return finishCheckout(start, res)
	}

	step("click Checkout")
	if err = clickFirstVisible(page, checkoutCandidates(), ms); err != nil {
		shot(page, o.ShotDir, "no-checkout-btn")
		res.Error = "checkout button not found: " + err.Error()
		return finishCheckout(start, res)
	}
	step("on checkout form")

	fillShipping(page, o.Profile)
	step("shipping filled")
	fillPayment(page, o.Profile)
	step("payment filled (card **** " + last4(o.Profile.CardNumber) + ")")

	snap := shot(page, o.ShotDir, "review")
	res.Screenshot = snap

	if !o.PlaceOrder {
		res.OK = true
		res.DryRunStopped = true
		step("DRY RUN — review the browser window/screenshot, then place the order yourself (or re-run with --place-order)")
		return finishCheckout(start, res)
	}

	step("click Place Order")
	if err = clickFirstVisible(page, placeOrderCandidates(), ms); err != nil {
		res.Error = "place-order button not found: " + err.Error()
		return finishCheckout(start, res)
	}
	res.OK = true
	res.OrderPlaced = true
	step("order submitted — verify confirmation in browser")
	shot(page, o.ShotDir, "confirmation")
	return finishCheckout(start, res)
}

func finishCheckout(start time.Time, r CheckoutResult) CheckoutResult {
	r.DurationMs = time.Since(start).Milliseconds()
	return r
}

func addToCartCandidates() []string {
	return []string{
		`button:has-text("Add to Cart")`,
		`button:has-text("Add to Bag")`,
		`[data-test="add-to-cart"]`,
		`#add-to-cart`,
		`.add-to-cart button`,
	}
}

func checkoutCandidates() []string {
	return []string{
		`button:has-text("Checkout")`,
		`a:has-text("Checkout")`,
		`[data-test="checkout"]`,
		`.checkout-button`,
	}
}

func placeOrderCandidates() []string {
	return []string{
		`button:has-text("Place Order")`,
		`button:has-text("Submit Order")`,
		`button:has-text("Pay Now")`,
		`[data-test="place-order"]`,
	}
}

// clickFirstVisible tries each selector, clicking the first that is visible.
func clickFirstVisible(page playwright.Page, candidates []string, timeoutMs float64) error {
	var lastErr error
	for _, sel := range candidates {
		loc := page.Locator(sel)
		if err := loc.First().WaitFor(playwright.LocatorWaitForOptions{
			State:   playwright.WaitForSelectorStateVisible,
			Timeout: playwright.Float(4000),
		}); err != nil {
			lastErr = err
			continue
		}
		if err := loc.First().Click(playwright.LocatorClickOptions{
			Timeout: playwright.Float(timeoutMs),
		}); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no candidates matched")
	}
	return lastErr
}

// fillFirst tries selectors in order, filling the first that exists.
// Reports whether anything was filled so callers can skip slow fallbacks.
func fillFirst(page playwright.Page, candidates []string, value string) bool {
	if value == "" {
		return true // nothing to do counts as done
	}
	for _, sel := range candidates {
		loc := page.Locator(sel).First()
		if err := loc.WaitFor(playwright.LocatorWaitForOptions{
			State:   playwright.WaitForSelectorStateAttached,
			Timeout: playwright.Float(800),
		}); err != nil {
			continue
		}
		if err := loc.Fill(value, playwright.LocatorFillOptions{
			Timeout: playwright.Float(3000),
		}); err != nil {
			continue
		}
		return true
	}
	return false
}

func fillShipping(page playwright.Page, p Profile) {
	fillFirst(page, []string{`input[name="email"]`, `input[type="email"]`, `#email`}, p.Email)
	fillFirst(page, []string{`input[name="firstName"]`, `input[name="first_name"]`, `#first-name`, `[autocomplete="given-name"]`}, p.FirstName)
	fillFirst(page, []string{`input[name="lastName"]`, `input[name="last_name"]`, `#last-name`, `[autocomplete="family-name"]`}, p.LastName)
	fillFirst(page, []string{`input[name="phone"]`, `input[type="tel"]`, `[autocomplete="tel"]`}, p.Phone)
	fillFirst(page, []string{`input[name="address1"]`, `input[name="street-address"]`, `[autocomplete="address-line1"]`, `#address-1`}, p.Address1)
	fillFirst(page, []string{`input[name="address2"]`, `[autocomplete="address-line2"]`}, p.Address2)
	fillFirst(page, []string{`input[name="city"]`, `[autocomplete="address-level2"]`, `#city`}, p.City)
	fillFirst(page, []string{`input[name="state"]`, `[autocomplete="address-level1"]`, `#state`}, p.State)
	fillFirst(page, []string{`input[name="zip"]`, `input[name="postalCode"]`, `[autocomplete="postal-code"]`, `#zip`}, p.Zip)
}

// fillPayment fills card fields on the main frame, then inside common
// hosted-field iframes (Salesforce/Cybersource tokenize card inputs).
func fillPayment(page playwright.Page, p Profile) {
	if p.CardNumber == "" {
		return
	}
	num := []string{`input[name="cardNumber"]`, `input[name="card-number"]`, `[autocomplete="cc-number"]`, `#card-number`}
	exp := []string{`input[name="expiry"]`, `input[name="expDate"]`, `[autocomplete="cc-exp"]`, `#exp-date`}
	cvc := []string{`input[name="cvv"]`, `input[name="cvc"]`, `[autocomplete="cc-csc"]`, `#cvv`}
	name := []string{`input[name="cardName"]`, `[autocomplete="cc-name"]`, `#card-name`}

	okNum := fillFirst(page, num, digitOnly(p.CardNumber))
	okExp := fillFirst(page, exp, expCombined(p))
	okCvc := fillFirst(page, cvc, p.CVV)
	okName := fillFirst(page, name, p.CardName)

	// Hosted iframes fallback — only for fields the main frame missed.
	if okNum && okExp && okCvc && okName {
		return
	}
	for _, frame := range cardFrameCandidates() {
		fl := page.FrameLocator(frame)
		if !okNum {
			tryFrameFill(fl, num, digitOnly(p.CardNumber))
		}
		if !okExp {
			tryFrameFill(fl, exp, expCombined(p))
		}
		if !okCvc {
			tryFrameFill(fl, cvc, p.CVV)
		}
		if !okName {
			tryFrameFill(fl, name, p.CardName)
		}
	}
}

func cardFrameCandidates() []string {
	return []string{
		`iframe[title*="card" i]`,
		`iframe[name*="card" i]`,
		`iframe[src*="cybersource" i]`,
		`iframe[src*="payment" i]`,
	}
}

func tryFrameFill(fl playwright.FrameLocator, candidates []string, value string) {
	if value == "" {
		return
	}
	for _, sel := range candidates {
		loc := fl.Locator(sel).First()
		_ = loc.WaitFor(playwright.LocatorWaitForOptions{
			State:   playwright.WaitForSelectorStateAttached,
			Timeout: playwright.Float(500),
		})
		_ = loc.Fill(value, playwright.LocatorFillOptions{
			Timeout: playwright.Float(800),
		})
	}
}

func expCombined(p Profile) string {
	m := strings.TrimSpace(p.ExpMonth)
	y := strings.TrimSpace(p.ExpYear)
	if len(y) == 4 {
		y = y[2:]
	}
	if m != "" && y != "" {
		return m + " / " + y
	}
	return m + y
}

func last4(pan string) string {
	d := digitOnly(pan)
	if len(d) >= 4 {
		return d[len(d)-4:]
	}
	return "????"
}

func isQueuePage(page playwright.Page) bool {
	body, err := page.Content()
	if err != nil {
		return false
	}
	return looksLikeBotWall(strings.ToLower(body))
}

// waitForQueuePass polls until the queue/CAPTCHA page is gone (human solves
// it in the headed window) or the deadline hits.
func waitForQueuePass(page playwright.Page, max time.Duration) error {
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		time.Sleep(5 * time.Second)
		if !isQueuePage(page) {
			return nil
		}
	}
	return fmt.Errorf("queue still active after %v — solve it in the browser and re-run", max)
}

func cartURLFor(productURL string) string {
	if i := strings.Index(productURL, "/product/"); i > 0 {
		return productURL[:i] + "/cart"
	}
	return "https://www.pokemoncenter.com/cart"
}

func shot(page playwright.Page, dir, name string) string {
	path := filepath.Join(dir, fmt.Sprintf("%s-%d.png", name, time.Now().Unix()))
	_, _ = page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String(path)})
	return path
}

// ---- async run registry (used by the REST API) ----

type checkoutRun struct {
	ID      string         `json:"id"`
	Product string         `json:"product"`
	DryRun  bool           `json:"dry_run"`
	State   string         `json:"state"` // running | done | error
	Result  CheckoutResult `json:"result,omitempty"`
}

var checkoutRuns = struct {
	sync.Mutex
	runs map[string]*checkoutRun
}{runs: map[string]*checkoutRun{}}

func startCheckoutRun(productName, productURL string, p Profile, headed, placeOrder bool) *checkoutRun {
	id := fmt.Sprintf("co-%d", time.Now().UnixNano())
	r := &checkoutRun{ID: id, Product: productName, DryRun: !placeOrder, State: "running"}
	checkoutRuns.Lock()
	checkoutRuns.runs[id] = r
	checkoutRuns.Unlock()
	go func() {
		res := RunCheckout(CheckoutOptions{
			ProductURL: productURL,
			Profile:    p,
			Headed:     headed,
			PlaceOrder: placeOrder,
		})
		checkoutRuns.Lock()
		r.Result = res
		if res.OK {
			r.State = "done"
		} else {
			r.State = "error"
		}
		checkoutRuns.Unlock()
	}()
	return r
}
