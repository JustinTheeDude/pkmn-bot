# pokebot-go — fast Pokemon Center restock monitor (Go)

Concurrent poller + instant alerts for sealed Pokemon Center drops.
Zero dependencies, stdlib only.

## Why this design

`pokemoncenter.com` sits behind Queue-it + Akamai/Incapsula. Plain HTTP
polling gets `403`, and full auto-checkout gets queue-blocked or banned
without a real browser session + residential proxies. Paid bots (Stellar,
etc.) charge monthly for that and still tell you to sit at your PC.

So this bot does the part that wins: **detect the flip in seconds and get
you to checkout instantly** in your already-logged-in browser.

## Setup

1. Edit `config.json`:
   - paste the exact product URLs you want (Pokemon Center product pages)
   - set `check_interval_sec` (20s is safe; <10s risks IP rate-limit)
   - add a Discord webhook URL to get phone push alerts
2. Run:

```sh
cd ~/code/pokebot-go
go vet ./...
go run . --once        # single test pass
go run .               # live monitor
```

3. Dashboard + REST API (React frontend, Go backend):

```sh
cd ~/code/pokebot-go/web && npm install && npm run build
cd ~/code/pokebot-go && ./pokebot --ui 127.0.0.1:8080
# open http://127.0.0.1:8080
```

Dev mode with hot reload: `cd web && npm run dev` (proxies `/api` to `:8080`
— keep the Go server running with `--ui 127.0.0.1:8080`).

REST endpoints (`api.go`, `drops.go`):

```
GET    /api/status            monitor snapshot
GET    /api/settings          monitor settings
PUT    /api/settings          update settings (interval ≥ 10 enforced)
GET    /api/products          list watched products
POST   /api/products          {name, url} -> create (201)
DELETE /api/products/{index}  delete by index
GET    /api/profile           masked profile (never full PAN/CVV)
PUT    /api/profile           update profile (blank card fields = keep)
POST   /api/checkout/{index}  start browser checkout {headed, place_order}
GET    /api/checkout/runs     list checkout runs
GET    /api/checkout/runs/{id} one run + steps
GET    /api/drops             list scheduled drops
POST   /api/drops             schedule a drop (RFC3339 fire_at)
GET    /api/drops/{id}        one drop job
DELETE /api/drops/{id}        cancel a scheduled/running drop
```

## Playwright checkout + scheduled drops

```sh
# one-off browser checkout, dry-run by default (fills everything, stops
# before spending). Add --place-order to really buy.
./pokebot --checkout 0
./pokebot --checkout 0 --place-order
```

The dashboard has ⏰ Scheduled drops: name + URL + exact time, tries (default
5) and retry gap ms (default 2000, min 500). At fire time it fast-checks for
Add to Cart; first hit opens ONE headed browser checkout, full miss kills the
job (`dead`). First run needs ` headed` browser so you can pass the Queue-it
queue/CAPTCHA once — login persists in `browser-profile/storage.json`.

React source lives in `web/src` (`App.jsx`, `api.js`, dashboard panels in
`components/`, shadcn-style primitives in `components/ui/` + Tailwind).
`GET /` serves `web/dist` when built.
The API binds 127.0.0.1 only and `profile.json` stays 0600, gitignored.

## Discovery auto-buy (opt-in)

URL discovery (`discover.auto_buy` in settings, or the Discovery tab) can go
one step further than watchlist + Discord ping:

- `off` (default) — matches are added to Products, nothing else happens.
- `dry-run` — each match is fast stock-checked; in-stock hits open ONE headed
  browser checkout using `profile.json`, stopping before payment.
- `live` — same, but actually places the order (spends real money).

OOS/blocked matches are skipped and stay in the watchlist for the regular
monitor. Keep a headed browser logged in so the Queue-it queue/CAPTCHA can be
passed once — the login persists in `browser-profile/storage.json`.

## Getting a Discord webhook (2 min, for phone alerts)

1. Discord server → channel settings → Integrations → Webhooks → New
2. Copy URL → paste as `discord_webhook` in `config.json`

## Tips that actually matter

- **Stay logged into Pokemon Center** in your main browser with payment +
  shipping saved before the drop.
- Leave `auto_open_browser: true` — the bot opens the product tab the
  second it flips so you just hit Add to Cart.
- One product per worker (goroutine); workers are staggered to avoid
  synchronized bursts.
- If logs show `QUEUED/BLOCKED ... 403` constantly, that's the bot wall —
  normal. The monitor keeps retrying; your browser session is what
  checks out.
- Don't set interval under ~15s from one IP or you'll get temp-limited.

## What it detects

Parses the Salesforce Commerce markup for `Add to Cart` enabled vs
`Out of stock / Sold out / Notify me when available` + disabled-button
states. Prints `QUEUED/BLOCKED` separately so a queue page never fires a
false IN-STOCK alert.

## Disclaimer

For personal use monitoring public product pages. Not affiliated with
Pokemon. Respect Pokemon Center ToS / rate limits — this polls product
pages, it does not bypass queues or CAPTCHAs.
