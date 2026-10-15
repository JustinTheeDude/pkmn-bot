import * as React from 'react'
import { Radar } from 'lucide-react'
import { api } from '../api.js'
import { Button } from './ui/button.jsx'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card.jsx'
import { Input } from './ui/input.jsx'
import { Label } from './ui/label.jsx'
import { Switch } from './ui/switch.jsx'
import { Badge } from './ui/badge.jsx'

export function DiscoveryPanel() {
  const [cfg, setCfg] = React.useState(null)
  const [meta, setMeta] = React.useState({})
  const [found, setFound] = React.useState([])
  const [msg, setMsg] = React.useState('')

  const refresh = React.useCallback(async () => {
    try {
      const d = await api.getDiscover()
      setCfg(d.config)
      setMeta(d)
      const f = await api.listDiscovered()
      setFound(f)
    } catch { /* offline */ }
  }, [])

  React.useEffect(() => {
    refresh()
    const t = setInterval(refresh, 15000)
    return () => clearInterval(t)
  }, [refresh])

  async function save() {
    setMsg('Saving…')
    try {
      const kws = Array.isArray(cfg.keywords)
        ? cfg.keywords
        : String(cfg.keywords || '').split(',').map((s) => s.trim()).filter(Boolean)
      const saved = await api.saveDiscover({ enabled: !!cfg.enabled, interval_min: Number(cfg.interval_min) || 5, keywords: kws, auto_buy: cfg.auto_buy || 'off' })
      setCfg(saved)
      setMsg('Saved — takes effect within a minute')
    } catch (e) {
      setMsg(e.message)
    }
  }

  if (!cfg) return <p className="text-sm text-muted-foreground">Loading discovery…</p>
  const set = (k, v) => setCfg((c) => ({ ...c, [k]: v }))

  return (
    <div className="grid gap-4 xl:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2"><Radar className="h-5 w-5" /> Sitemap discovery</CardTitle>
          <CardDescription>
            Polls Pokémon Center&apos;s product sitemap for new URLs matching keywords. Matches auto-add + Discord ping.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between rounded-md border p-3">
            <div>
              <Label htmlFor="disc-enabled">Poll sitemap</Label>
              <p className="text-xs text-muted-foreground">Every few minutes · first run seeds catalog</p>
            </div>
            <Switch id="disc-enabled" checked={!!cfg.enabled} onCheckedChange={(v) => set('enabled', v)} />
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label>Poll every (minutes)</Label>
              <Input type="number" min="2" value={cfg.interval_min} onChange={(e) => set('interval_min', e.target.value)} />
            </div>
            <div className="space-y-1.5">
              <Label>Keywords (comma separated)</Label>
              <Input
                value={Array.isArray(cfg.keywords) ? cfg.keywords.join(', ') : cfg.keywords}
                onChange={(e) => set('keywords', e.target.value.split(','))}
              />
            </div>
          </div>
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <Badge variant="secondary">known: {meta.known_count ?? '—'}</Badge>
            <Badge variant="secondary">last new: {meta.last_new ?? 0}</Badge>
            <span className="truncate">last check: {meta.last_check ? new Date(meta.last_check).toLocaleString() : '—'}</span>
          </div>
          <div className="space-y-1.5">
            <Label>Auto-buy discovered URLs</Label>
            <div className="grid grid-cols-3 gap-1 rounded-md bg-muted p-1">
              {[
                { id: 'off', label: 'Off' },
                { id: 'dry-run', label: 'Dry-run' },
                { id: 'live', label: 'Live' },
              ].map((o) => (
                <button
                  key={o.id}
                  onClick={() => set('auto_buy', o.id)}
                  className={`rounded-sm px-3 py-1.5 text-sm font-medium transition-all ${
                    (cfg.auto_buy || 'off') === o.id
                      ? 'bg-background text-foreground shadow-sm'
                      : 'text-muted-foreground hover:text-foreground'
                  }`}
                >
                  {o.label}
                </button>
              ))}
            </div>
            <p className="text-xs text-muted-foreground">
              {(!cfg.auto_buy || cfg.auto_buy === 'off') && 'Matches are watchlisted + pinged on Discord. No checkout.'}
              {cfg.auto_buy === 'dry-run' && 'In-stock hits open a headed browser checkout with your profile. Stops before spending.'}
              {cfg.auto_buy === 'live' && 'In-stock hits PLACE THE ORDER with your saved card. Real money.'}
            </p>
          </div>
          {(cfg.auto_buy === 'live') && (
            <div className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
              Live auto-buy spends money the moment a discovered URL shows Add to Cart. Keep a headed browser logged in and verify your profile first.
            </div>
          )}
          <Button onClick={save}>Save discovery</Button>
          {msg && <p className="text-sm text-muted-foreground">{msg}</p>}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Newly matched URLs ({found.length})</CardTitle>
          <CardDescription>Latest sitemap hits. Auto-added to Products on the backend.</CardDescription>
        </CardHeader>
        <CardContent>
          {found.length === 0 ? (
            <p className="text-sm text-muted-foreground">Nothing new — catalog is quiet.</p>
          ) : (
            <ul className="space-y-2">
              {found.map((u) => (
                <li key={u} className="break-all rounded-md bg-muted p-2 font-mono text-xs">{u}</li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
