import * as React from 'react'
import { CalendarClock, X } from 'lucide-react'
import { api } from '../api.js'
import { Button } from './ui/button.jsx'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card.jsx'
import { Input } from './ui/input.jsx'
import { Label } from './ui/label.jsx'
import { Badge } from './ui/badge.jsx'
import { Switch } from './ui/switch.jsx'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from './ui/table.jsx'

function stateVariant(s) {
  switch (s) {
    case 'done': return 'success'
    case 'running': return 'default'
    case 'scheduled': return 'secondary'
    case 'dead':
    case 'cancelled': return 'destructive'
    default: return 'outline'
  }
}

export function DropsPanel() {
  const [jobs, setJobs] = React.useState([])
  const [form, setForm] = React.useState({ name: '', url: '', fire_at: '', attempts: 5, interval_ms: 2000, place_order: false })
  const [msg, setMsg] = React.useState('')

  const refresh = React.useCallback(async () => {
    try {
      const list = await api.listDrops()
      list.sort((a, b) => new Date(b.created_at) - new Date(a.created_at))
      setJobs(list)
    } catch { /* offline */ }
  }, [])

  React.useEffect(() => {
    refresh()
    const t = setInterval(refresh, 3000)
    return () => clearInterval(t)
  }, [refresh])

  const set = (k, v) => setForm((f) => ({ ...f, [k]: v }))

  async function schedule() {
    if (!form.name || !form.url || !form.fire_at) return setMsg('Name + URL + time required')
    try {
      await api.scheduleDrop({
        name: form.name,
        url: form.url,
        fire_at: new Date(form.fire_at).toISOString(),
        max_attempts: Number(form.attempts) || 5,
        retry_interval_ms: Number(form.interval_ms) || 2000,
        place_order: !!form.place_order,
        headed: true,
      })
      setForm({ name: '', url: '', fire_at: '', attempts: 5, interval_ms: 2000, place_order: false })
      setMsg('')
      refresh()
    } catch (e) {
      setMsg(e.message)
    }
  }

  async function cancel(id) {
    try {
      await api.cancelDrop(id)
      refresh()
    } catch (e) {
      setMsg(e.message)
    }
  }

  return (
    <div className="grid gap-4 xl:grid-cols-5">
      <Card className="xl:col-span-3">
        <CardHeader>
          <CardTitle className="flex items-center gap-2"><CalendarClock className="h-5 w-5" /> Drop jobs</CardTitle>
          <CardDescription>Fires at the exact time, retries for Add to Cart, then opens one browser checkout. Misses exhaust → dead.</CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Job</TableHead>
                <TableHead>State</TableHead>
                <TableHead className="hidden md:table-cell">Fire at</TableHead>
                <TableHead className="text-right">Action</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {jobs.map((j) => (
                <TableRow key={j.id}>
                  <TableCell>
                    <p className="font-medium">{j.name}</p>
                    <p className="font-mono text-xs text-muted-foreground">tries {j.attempts}/{j.max_attempts} · {j.note || ''}</p>
                  </TableCell>
                  <TableCell><Badge variant={stateVariant(j.state)}>{j.state}</Badge></TableCell>
                  <TableCell className="hidden whitespace-nowrap text-xs text-muted-foreground md:table-cell">
                    {new Date(j.fire_at).toLocaleString()}
                  </TableCell>
                  <TableCell className="text-right">
                    {(j.state === 'scheduled' || j.state === 'running') && (
                      <Button variant="ghost" size="sm" onClick={() => cancel(j.id)}>
                        <X className="h-3.5 w-3.5" /> Cancel
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
              {jobs.length === 0 && (
                <TableRow>
                  <TableCell colSpan={4} className="hand text-center text-lg text-muted-foreground">no drops scheduled</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <Card className="h-fit xl:col-span-2">
        <CardHeader>
          <CardTitle>Schedule a drop</CardTitle>
          <CardDescription>Leave place-order off for dry-run.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="space-y-1.5">
            <Label>Product name</Label>
            <Input placeholder="Prismatic Evolutions ETB" value={form.name} onChange={(e) => set('name', e.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label>Product URL</Label>
            <Input placeholder="https://www.pokemoncenter.com/product/…" value={form.url} onChange={(e) => set('url', e.target.value)} />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="col-span-2 space-y-1.5">
              <Label>Drop time (local)</Label>
              <Input type="datetime-local" value={form.fire_at} onChange={(e) => set('fire_at', e.target.value)} />
            </div>
            <div className="space-y-1.5">
              <Label>Tries</Label>
              <Input type="number" min="1" max="20" value={form.attempts} onChange={(e) => set('attempts', e.target.value)} />
            </div>
            <div className="space-y-1.5">
              <Label>Gap ms (≥500)</Label>
              <Input type="number" min="500" step="500" value={form.interval_ms} onChange={(e) => set('interval_ms', e.target.value)} />
            </div>
          </div>
          <div className="wobble-sm flex items-center justify-between border-2 border-border p-3">
            <Label htmlFor="place-order">Actually place order</Label>
            <Switch id="place-order" checked={form.place_order} onCheckedChange={(v) => set('place_order', v)} />
          </div>
          <Button className="w-full" onClick={schedule}>Schedule drop</Button>
          {msg && <p className="text-sm text-destructive">{msg}</p>}
        </CardContent>
      </Card>
    </div>
  )
}
