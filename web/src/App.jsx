import * as React from 'react'
import { Package, Timer, Radar, CreditCard, TriangleAlert } from 'lucide-react'
import { api } from './api.js'
import { Sidebar, MobileNav, TopHeader, StatCard, NAV } from './components/layout.jsx'
import { ProductsPanel } from './components/products-panel.jsx'
import { DropsPanel } from './components/drops-panel.jsx'
import { DiscoveryPanel } from './components/discovery-panel.jsx'
import { SettingsPanel } from './components/settings-panel.jsx'
import { ProfilePanel } from './components/profile-panel.jsx'
import { CheckoutsPanel } from './components/checkouts-panel.jsx'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './components/ui/card.jsx'
import { Alert, AlertDescription, AlertTitle } from './components/ui/alert.jsx'
import { Badge } from './components/ui/badge.jsx'
import { Skeleton } from './components/ui/skeleton.jsx'
import './index.css'

function Overview({ status, products, drops, discovered, runs, onNavigate }) {
  const activeDrops = drops.filter((d) => d.state === 'scheduled' || d.state === 'running').length
  const stats = [
    { title: 'Watching', value: products.length, hint: status ? `every ${status.interval_sec}s` : '', icon: Package },
    { title: 'Active drops', value: activeDrops, hint: `${drops.length} total jobs`, icon: Timer },
    { title: 'Discovered URLs', value: discovered.length, hint: 'sitemap hits', icon: Radar },
    { title: 'Checkout runs', value: runs.length, hint: 'browser sessions', icon: CreditCard },
  ]
  return (
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {stats.map((s) => <StatCard key={s.title} {...s} />)}
      </div>
      <div className="grid gap-4 xl:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Next drops</CardTitle>
            <CardDescription>Upcoming scheduled jobs.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            {drops.slice(0, 4).map((d) => (
              <button key={d.id} onClick={() => onNavigate('drops')} className="wobble-sm flex w-full items-center justify-between border-2 border-border p-3 text-left transition-colors hover:bg-accent/40">
                <div className="min-w-0">
                  <p className="truncate text-sm font-bold">{d.name}</p>
                  <p className="font-mono text-xs text-muted-foreground">{new Date(d.fire_at).toLocaleString()}</p>
                </div>
                <Badge variant={d.state === 'scheduled' ? 'secondary' : 'default'}>{d.state}</Badge>
              </button>
            ))}
            {drops.length === 0 && <p className="hand text-lg text-muted-foreground">no drops scheduled — plan the next hype drop!</p>}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Watchlist snapshot</CardTitle>
            <CardDescription>First few watched products.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            {products.slice(0, 4).map((p, i) => (
              <button key={i} onClick={() => onNavigate('products')} className="wobble-sm flex w-full items-center justify-between border-2 border-border p-3 text-left transition-colors hover:bg-accent/40">
                <p className="truncate text-sm font-bold">{p.name}</p>
                <Badge variant="secondary">watching</Badge>
              </button>
            ))}
            {products.length === 0 && <p className="hand text-lg text-muted-foreground">watchlist is empty…</p>}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

export default function App() {
  const [tab, setTab] = React.useState('overview')
  const [status, setStatus] = React.useState(null)
  const [settings, setSettings] = React.useState(null)
  const [products, setProducts] = React.useState([])
  const [profile, setProfile] = React.useState(null)
  const [drops, setDrops] = React.useState([])
  const [discovered, setDiscovered] = React.useState([])
  const [runs, setRuns] = React.useState([])
  const [connected, setConnected] = React.useState(false)
  const [loadError, setLoadError] = React.useState('')

  const refreshProducts = React.useCallback(async () => {
    try { setProducts(await api.listProducts()) } catch { /* keep stale */ }
  }, [])

  const refreshSecondary = React.useCallback(async () => {
    try {
      const [d, f, r, s] = await Promise.all([
        api.listDrops().catch(() => []),
        api.listDiscovered().catch(() => []),
        api.checkoutRuns().catch(() => []),
        api.status().catch(() => null),
      ])
      setDrops(Array.isArray(d) ? d : [])
      setDiscovered(Array.isArray(f) ? f : [])
      setRuns(Array.isArray(r) ? r : [])
      if (s) { setStatus(s); setConnected(true) }
    } catch { /* offline */ }
  }, [])

  React.useEffect(() => {
    let stop = false
    async function load() {
      try {
        const [s, st, p, m] = await Promise.all([
          api.status(), api.getSettings(), api.listProducts(), api.getProfile(),
        ])
        if (stop) return
        setStatus(s); setSettings(st); setProducts(p); setProfile(m)
        setConnected(true)
        setLoadError('')
      } catch (e) {
        if (!stop) { setLoadError('API unreachable — is the Go server running? (' + e.message + ')'); setConnected(false) }
      }
      refreshSecondary()
    }
    load()
    const t = setInterval(async () => {
      try {
        setStatus(await api.status())
        setConnected(true)
      } catch { setConnected(false) }
      refreshSecondary()
    }, 10000)
    return () => { stop = true; clearInterval(t) }
  }, [refreshSecondary])

  const counts = {
    products: products.length,
    drops: drops.filter((d) => d.state === 'scheduled' || d.state === 'running').length,
    discovery: discovered.length,
    checkouts: runs.length,
  }
  const activeLabel = NAV.find((n) => n.id === tab)?.label ?? 'Overview'

  return (
    <div className="flex min-h-screen">
      <Sidebar active={tab} onNavigate={setTab} counts={counts} />
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="mx-auto w-full max-w-6xl flex-1 space-y-4 p-4 sm:p-6">
          <TopHeader status={status} connected={connected} />
          <MobileNav active={tab} onNavigate={setTab} />

          {loadError && (
            <Alert variant="destructive">
              <TriangleAlert className="h-4 w-4" />
              <AlertTitle>Backend offline</AlertTitle>
              <AlertDescription>{loadError}</AlertDescription>
            </Alert>
          )}

          {!settings && !loadError ? (
            <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
              {[0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-24" />)}
            </div>
          ) : (
            <>
              <p className="font-display text-xs font-bold uppercase tracking-wide text-muted-foreground">{activeLabel}</p>
              {tab === 'overview' && (
                <Overview status={status} products={products} drops={drops} discovered={discovered} runs={runs} onNavigate={setTab} />
              )}
              {tab === 'products' && <ProductsPanel items={products} onChange={refreshProducts} />}
              {tab === 'drops' && <DropsPanel />}
              {tab === 'discovery' && <DiscoveryPanel />}
              {tab === 'checkouts' && <CheckoutsPanel />}
              {tab === 'settings' && settings && <SettingsPanel initial={settings} onSaved={setSettings} />}
              {tab === 'profile' && profile && (
                <ProfilePanel key={(profile.card_last4 || '') + (profile.email || '')} initial={profile} />
              )}
            </>
          )}

          <p className="hand pt-2 text-center text-lg text-muted-foreground">
            {status ? `watching ${status.watching} · every ${status.interval_sec}s · ${status.note || 'live'}` : 'connecting…'} · localhost only
          </p>
        </div>
      </div>
    </div>
  )
}
