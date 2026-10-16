import * as React from 'react'
import { api } from '../api.js'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card.jsx'
import { Input } from './ui/input.jsx'
import { Label } from './ui/label.jsx'
import { Button } from './ui/button.jsx'
import { Switch } from './ui/switch.jsx'
import { Badge } from './ui/badge.jsx'

export function SettingsPanel({ initial, onSaved }) {
  const [form, setForm] = React.useState({
    check_interval_sec: initial?.check_interval_sec ?? 20,
    jitter_ms: initial?.jitter_ms ?? 3000,
    timeout_sec: initial?.timeout_sec ?? 15,
    discord_webhook: initial?.discord_webhook ?? '',
    auto_open_browser: initial?.auto_open_browser ?? true,
    beep_on_hit: initial?.beep_on_hit ?? true,
    discover: initial?.discover ?? { enabled: true, interval_min: 5, keywords: [] },
  })
  const [msg, setMsg] = React.useState('')
  const [busy, setBusy] = React.useState(false)

  const set = (k, v) => setForm((f) => ({ ...f, [k]: v }))

  async function save() {
    setBusy(true)
    setMsg('Saving…')
    try {
      const saved = await api.saveSettings({
        ...form,
        check_interval_sec: Number(form.check_interval_sec),
        timeout_sec: Number(form.timeout_sec),
        jitter_ms: Number(form.jitter_ms),
        discover: form.discover,
      })
      setMsg('Saved — restart monitor to apply interval')
      onSaved?.(saved)
    } catch (e) {
      setMsg(e.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="grid gap-4 xl:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>Monitor</CardTitle>
          <CardDescription>Poll cadence. Keep interval ≥ 10s to avoid IP rate-limits.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="grid grid-cols-3 gap-3">
            <div className="space-y-1.5">
              <Label>Interval (s)</Label>
              <Input type="number" min="10" value={form.check_interval_sec} onChange={(e) => set('check_interval_sec', e.target.value)} />
            </div>
            <div className="space-y-1.5">
              <Label>Jitter (ms)</Label>
              <Input type="number" min="0" value={form.jitter_ms} onChange={(e) => set('jitter_ms', e.target.value)} />
            </div>
            <div className="space-y-1.5">
              <Label>Timeout (s)</Label>
              <Input type="number" min="5" value={form.timeout_sec} onChange={(e) => set('timeout_sec', e.target.value)} />
            </div>
          </div>
          <div className="space-y-1.5">
            <Label>Discord webhook</Label>
            <Input placeholder="https://discord.com/api/webhooks/…" value={form.discord_webhook} onChange={(e) => set('discord_webhook', e.target.value)} />
          </div>
          <div className="wobble-sm flex items-center justify-between border-2 border-border p-3">
            <div>
              <Label>Auto-open browser on hit</Label>
              <p className="text-xs text-muted-foreground">Opens product tab the second it flips</p>
            </div>
            <Switch checked={!!form.auto_open_browser} onCheckedChange={(v) => set('auto_open_browser', v)} />
          </div>
          <div className="wobble-sm flex items-center justify-between border-2 border-border p-3">
            <div>
              <Label>Beep on hit</Label>
              <p className="text-xs text-muted-foreground">Terminal bell + macOS Glass sound</p>
            </div>
            <Switch checked={!!form.beep_on_hit} onCheckedChange={(v) => set('beep_on_hit', v)} />
          </div>
          <Button onClick={save} disabled={busy}>{busy ? 'Saving…' : 'Save settings'}</Button>
          {msg && <p className="text-sm text-muted-foreground">{msg}</p>}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Rate-limit safety</CardTitle>
          <CardDescription>Why these floors exist.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3 text-sm text-muted-foreground">
          <div className="wobble-sm flex items-start justify-between border-2 border-border p-3">
            <span>Poll interval floor</span><Badge>≥ 10s</Badge>
          </div>
          <div className="wobble-sm flex items-start justify-between border-2 border-border p-3">
            <span>HTTP timeout floor</span><Badge>≥ 5s</Badge>
          </div>
          <div className="wobble-sm flex items-start justify-between border-2 border-border p-3">
            <span>Drop retry gap floor</span><Badge>≥ 500ms</Badge>
          </div>
          <p className="leading-relaxed">
            Pokémon Center sits behind Queue-it + Akamai. Faster polling doesn&apos;t beat the wall —
            it gets your IP temp-limited. One worker per product, staggered 400ms apart.
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
