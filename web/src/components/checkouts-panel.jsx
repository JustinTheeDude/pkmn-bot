import * as React from 'react'
import { api } from '../api.js'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card.jsx'
import { Badge } from './ui/badge.jsx'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from './ui/table.jsx'

export function CheckoutsPanel() {
  const [runs, setRuns] = React.useState([])

  const refresh = React.useCallback(async () => {
    try {
      const list = await api.checkoutRuns()
      setRuns(Array.isArray(list) ? list : [])
    } catch { /* offline */ }
  }, [])

  React.useEffect(() => {
    refresh()
    const t = setInterval(refresh, 4000)
    return () => clearInterval(t)
  }, [refresh])

  return (
    <Card>
      <CardHeader>
        <CardTitle>Browser checkout runs ({runs.length})</CardTitle>
        <CardDescription>Dry-run by default — fills everything, stops before spending. Live tail, 4s refresh.</CardDescription>
      </CardHeader>
      <CardContent className="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Run</TableHead>
              <TableHead>State</TableHead>
              <TableHead className="hidden md:table-cell">Steps</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {runs.map((r) => (
              <TableRow key={r.id}>
                <TableCell>
                  <p className="font-medium">{r.product}</p>
                  <p className="font-mono text-xs text-muted-foreground">{r.id} · {r.dry_run ? 'dry-run' : 'LIVE order'}</p>
                </TableCell>
                <TableCell>
                  <Badge variant={r.state === 'done' ? 'success' : r.state === 'running' ? 'default' : 'destructive'}>
                    {r.state}
                  </Badge>
                </TableCell>
                <TableCell className="hidden max-w-[420px] md:table-cell">
                  <p className="truncate text-xs text-muted-foreground">
                    {(r.result?.steps || []).slice(-2).join(' → ') || '—'}
                  </p>
                </TableCell>
              </TableRow>
            ))}
            {runs.length === 0 && (
              <TableRow>
                <TableCell colSpan={3} className="text-center text-muted-foreground">
                  No runs yet — hit Dry-run on a product.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}
