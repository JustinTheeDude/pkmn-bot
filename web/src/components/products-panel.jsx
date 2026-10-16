import * as React from 'react'
import { Plus, Trash2, ExternalLink, Play } from 'lucide-react'
import { api } from '../api.js'
import { Button } from './ui/button.jsx'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card.jsx'
import { Input } from './ui/input.jsx'
import { Label } from './ui/label.jsx'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from './ui/table.jsx'

export function ProductsPanel({ items, onChange }) {
  const [name, setName] = React.useState('')
  const [url, setUrl] = React.useState('')
  const [msg, setMsg] = React.useState('')
  const [busy, setBusy] = React.useState(false)
  const [checkoutBusy, setCheckoutBusy] = React.useState(null)

  async function add() {
    if (!name || !url) return setMsg('Name + URL required')
    setBusy(true)
    setMsg('')
    try {
      await api.addProduct({ name, url })
      setName('')
      setUrl('')
      onChange?.()
    } catch (e) {
      setMsg(e.message)
    } finally {
      setBusy(false)
    }
  }

  async function remove(i) {
    try {
      await api.deleteProduct(i)
      onChange?.()
    } catch (e) {
      setMsg(e.message)
    }
  }

  async function buyNow(i) {
    setCheckoutBusy(i)
    setMsg('Starting browser checkout (dry-run)…')
    try {
      const run = await api.startCheckout(i, { headed: true, place_order: false })
      setMsg(`Checkout ${run.id} started — watch the browser window`)
    } catch (e) {
      setMsg(e.message)
    } finally {
      setCheckoutBusy(null)
    }
  }

  return (
    <div className="grid gap-4 xl:grid-cols-5">
      <Card className="xl:col-span-3">
        <CardHeader>
          <CardTitle>Watched products</CardTitle>
          <CardDescription>{items.length} sealed item(s) polled concurrently. Restart monitor to apply interval changes.</CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Product</TableHead>
                <TableHead className="hidden md:table-cell">URL</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((x, i) => (
                <TableRow key={`${x.url}-${i}`}>
                  <TableCell>
                    <p className="font-medium">{x.name}</p>
                    <a href={x.url} target="_blank" rel="noreferrer" className="mt-0.5 flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground md:hidden">
                      <span className="truncate font-mono">open link</span>
                      <ExternalLink className="h-3 w-3" />
                    </a>
                  </TableCell>
                  <TableCell className="hidden max-w-[280px] md:table-cell">
                    <a href={x.url} target="_blank" rel="noreferrer" className="flex items-center gap-1 truncate font-mono text-xs text-muted-foreground hover:text-foreground">
                      <span className="truncate">{x.url}</span>
                      <ExternalLink className="h-3 w-3 shrink-0" />
                    </a>
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex justify-end gap-1">
                      <Button variant="outline" size="sm" onClick={() => buyNow(i)} disabled={checkoutBusy === i}>
                        <Play className="h-3 w-3" />
                        {checkoutBusy === i ? 'Starting…' : 'Dry-run'}
                      </Button>
                      <Button variant="ghost" size="sm" onClick={() => remove(i)} aria-label="Remove">
                        <Trash2 className="h-3.5 w-3.5 text-destructive" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {items.length === 0 && (
                <TableRow>
                  <TableCell colSpan={3} className="text-center text-muted-foreground">
                    No products yet — add your first URL.
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
          {msg && <p className="px-6 py-3 text-sm text-muted-foreground">{msg}</p>}
        </CardContent>
      </Card>

      <Card className="h-fit xl:col-span-2">
        <CardHeader>
          <CardTitle>Add product</CardTitle>
          <CardDescription>Paste the exact Pokémon Center product page URL.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="p-name">Name</Label>
            <Input id="p-name" placeholder="Prismatic Evolutions ETB" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="p-url">Product URL</Label>
            <Input id="p-url" placeholder="https://www.pokemoncenter.com/product/…" value={url} onChange={(e) => setUrl(e.target.value)} />
          </div>
          <Button className="w-full" onClick={add} disabled={busy}>
            <Plus className="h-4 w-4" /> {busy ? 'Adding…' : 'Add to watchlist'}
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
