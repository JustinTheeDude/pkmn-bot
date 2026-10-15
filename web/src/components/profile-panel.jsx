import * as React from 'react'
import { ShieldAlert } from 'lucide-react'
import { api } from '../api.js'
import { Button } from './ui/button.jsx'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card.jsx'
import { Input } from './ui/input.jsx'
import { Label } from './ui/label.jsx'
import { Alert, AlertDescription, AlertTitle } from './ui/alert.jsx'

function Field({ label, ...props }) {
  return (
    <div className="space-y-1.5">
      <Label>{label}</Label>
      <Input autoComplete="off" {...props} />
    </div>
  )
}

export function ProfilePanel({ initial }) {
  const [form, setForm] = React.useState({
    first_name: initial?.first_name ?? '',
    last_name: initial?.last_name ?? '',
    email: initial?.email ?? '',
    phone: initial?.phone ?? '',
    address1: initial?.address1 ?? '',
    address2: initial?.address2 ?? '',
    city: initial?.city ?? '',
    state: initial?.state ?? '',
    zip: initial?.zip ?? '',
    country: initial?.country ?? 'US',
    card_name: initial?.card_name ?? '',
    card_number: '',
    exp_month: initial?.exp_month ?? '',
    exp_year: initial?.exp_year ?? '',
    cvv: '',
  })
  const [hint, setHint] = React.useState(initial?.card_number_masked ?? '')
  const [msg, setMsg] = React.useState('')
  const [busy, setBusy] = React.useState(false)

  const set = (k, v) => setForm((f) => ({ ...f, [k]: v }))

  async function save() {
    setBusy(true)
    setMsg('Saving…')
    try {
      const saved = await api.saveProfile(form)
      setForm((f) => ({ ...f, card_number: '', cvv: '' }))
      setHint(saved.card_number_masked || '(no card saved)')
      setMsg('Saved locally — card fields cleared from page')
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
          <CardTitle>Shipping</CardTitle>
          <CardDescription>Used to prefill browser checkout. Stored in profile.json (0600).</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <Field label="First name" value={form.first_name} onChange={(e) => set('first_name', e.target.value)} />
            <Field label="Last name" value={form.last_name} onChange={(e) => set('last_name', e.target.value)} />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Email" value={form.email} onChange={(e) => set('email', e.target.value)} />
            <Field label="Phone" value={form.phone} onChange={(e) => set('phone', e.target.value)} />
          </div>
          <Field label="Address 1" value={form.address1} onChange={(e) => set('address1', e.target.value)} />
          <Field label="Address 2 (apt/suite)" value={form.address2} onChange={(e) => set('address2', e.target.value)} />
          <div className="grid grid-cols-3 gap-3">
            <Field label="City" value={form.city} onChange={(e) => set('city', e.target.value)} />
            <Field label="State" placeholder="CA" value={form.state} onChange={(e) => set('state', e.target.value)} />
            <Field label="ZIP" value={form.zip} onChange={(e) => set('zip', e.target.value)} />
          </div>
          <Field label="Country" value={form.country} onChange={(e) => set('country', e.target.value)} />
        </CardContent>
      </Card>

      <div className="space-y-4">
        <Alert variant="warning">
          <ShieldAlert className="h-4 w-4" />
          <AlertTitle>Plain-text storage</AlertTitle>
          <AlertDescription>
            Stored as plain text in <code className="font-mono">profile.json</code>. For real drops prefer your
            logged-in browser; use this only to prefill. Current: <span className="font-mono">{hint || '(no card)'}</span>
          </AlertDescription>
        </Alert>
        <Card>
          <CardHeader>
            <CardTitle>Payment</CardTitle>
            <CardDescription>Blank card fields = keep saved card.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <Field label="Name on card" value={form.card_name} onChange={(e) => set('card_name', e.target.value)} />
            <Field label={`Card number ${hint}`} inputMode="numeric" placeholder="leave blank to keep saved card" value={form.card_number} onChange={(e) => set('card_number', e.target.value)} />
            <div className="grid grid-cols-3 gap-3">
              <Field label="Exp MM" placeholder="08" value={form.exp_month} onChange={(e) => set('exp_month', e.target.value)} />
              <Field label="Exp YYYY" placeholder="2028" value={form.exp_year} onChange={(e) => set('exp_year', e.target.value)} />
              <div className="space-y-1.5">
                <Label>CVV</Label>
                <Input autoComplete="off" type="password" placeholder="blank = keep" value={form.cvv} onChange={(e) => set('cvv', e.target.value)} />
              </div>
            </div>
            <Button className="w-full" onClick={save} disabled={busy}>{busy ? 'Saving…' : 'Save shipping + payment locally'}</Button>
            {msg && <p className="text-sm text-muted-foreground">{msg}</p>}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
