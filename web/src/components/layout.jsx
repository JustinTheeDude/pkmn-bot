import * as React from 'react'
import {
  Zap, Package, Timer, Radar, CreditCard, Settings as SettingsIcon,
  LayoutDashboard, Moon, Sun, ShieldCheck, Activity,
} from 'lucide-react'
import { Button } from './ui/button.jsx'
import { Badge } from './ui/badge.jsx'
import { Card, CardContent } from './ui/card.jsx'
import { cn } from '../lib/utils.js'

export const NAV = [
  { id: 'overview', label: 'Overview', icon: LayoutDashboard },
  { id: 'products', label: 'Products', icon: Package },
  { id: 'drops', label: 'Scheduled drops', icon: Timer },
  { id: 'discovery', label: 'Discovery', icon: Radar },
  { id: 'checkouts', label: 'Checkouts', icon: CreditCard },
  { id: 'settings', label: 'Settings', icon: SettingsIcon },
  { id: 'profile', label: 'Profile', icon: ShieldCheck },
]

export function ThemeToggle() {
  const [dark, setDark] = React.useState(() => document.documentElement.classList.contains('dark'))
  React.useEffect(() => {
    document.documentElement.classList.toggle('dark', dark)
    try { localStorage.setItem('pokebot-theme', dark ? 'dark' : 'light') } catch { /* ignore */ }
  }, [dark])
  React.useEffect(() => {
    try {
      if (localStorage.getItem('pokebot-theme') === 'dark') setDark(true)
      else if (window.matchMedia?.('(prefers-color-scheme: dark)').matches) setDark(true)
    } catch { /* ignore */ }
  }, [])
  return (
    <Button variant="outline" size="icon" onClick={() => setDark((d) => !d)} aria-label="Toggle theme">
      {dark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
    </Button>
  )
}

export function Sidebar({ active, onNavigate, counts }) {
  return (
    <aside className="hidden w-60 shrink-0 flex-col gap-1 border-r-2 border-border bg-card/60 p-4 lg:flex">
      <div className="mb-4 flex items-center gap-3 px-1">
        <div className="wobble-sm doodle-shadow flex h-11 w-11 shrink-0 -rotate-3 items-center justify-center border-2 border-foreground/20 bg-primary text-primary-foreground">
          <Zap className="h-5 w-5" strokeWidth={2.6} />
        </div>
        <div>
          <p className="font-display text-base font-extrabold leading-none">pokebot-go</p>
          <p className="hand text-lg leading-none text-muted-foreground">restock monitor</p>
        </div>
      </div>
      {NAV.map((n) => {
        const Icon = n.icon
        const isActive = active === n.id
        const count = counts?.[n.id]
        return (
          <button
            key={n.id}
            onClick={() => onNavigate(n.id)}
            className={cn(
              'wobble-sm flex items-center gap-3 border-2 px-3 py-2 font-display text-sm font-semibold transition-all',
              isActive
                ? 'doodle-shadow border-foreground/20 bg-primary text-primary-foreground'
                : 'border-transparent text-muted-foreground hover:border-foreground/10 hover:bg-accent/50 hover:text-accent-foreground',
            )}
          >
            <Icon className="h-4 w-4 shrink-0" strokeWidth={2.4} />
            <span className="flex-1 text-left">{n.label}</span>
            {count != null && count > 0 && (
              <span
                className={cn(
                  'wobble-sm border px-1.5 text-xs font-bold',
                  isActive ? 'border-primary-foreground/30 bg-primary-foreground/20' : 'border-foreground/10 bg-muted',
                )}
              >
                {count}
              </span>
            )}
          </button>
        )
      })}
      <div className="wobble mt-auto border-2 border-dashed border-border bg-secondary/30 p-3 text-xs text-muted-foreground">
        <p className="flex items-center gap-1.5 font-display font-bold text-foreground">
          <ShieldCheck className="h-3.5 w-3.5" /> Localhost only
        </p>
        <p className="mt-1 leading-relaxed">
          API on <code className="font-mono">127.0.0.1</code> · profile is <code className="font-mono">0600</code> · never expose publicly.
        </p>
      </div>
    </aside>
  )
}

export function MobileNav({ active, onNavigate }) {
  return (
    <div className="flex gap-1.5 overflow-x-auto pb-1 lg:hidden">
      {NAV.map((n) => {
        const Icon = n.icon
        return (
          <button
            key={n.id}
            onClick={() => onNavigate(n.id)}
            className={cn(
              'wobble-sm flex shrink-0 items-center gap-1.5 border-2 px-3 py-2 font-display text-sm font-semibold',
              active === n.id
                ? 'doodle-shadow border-foreground/20 bg-primary text-primary-foreground'
                : 'border-border bg-card text-muted-foreground',
            )}
          >
            <Icon className="h-4 w-4" strokeWidth={2.4} />
            {n.label}
          </button>
        )
      })}
    </div>
  )
}

export function TopHeader({ status, connected }) {
  return (
    <header className="flex flex-wrap items-center gap-3">
      <div className="min-w-0 flex-1">
        <h1 className="squiggle inline-block font-display text-2xl font-extrabold tracking-tight">Mission control</h1>
        <p className="hand text-lg leading-tight text-muted-foreground">
          monitor + scheduled drops + checkout — all local
        </p>
      </div>
      <div className="flex items-center gap-2">
        <Badge variant={connected ? 'success' : 'destructive'} className="gap-1.5">
          <span className={cn('h-1.5 w-1.5 rounded-full', connected ? 'bg-emerald-500 animate-pulse' : 'bg-red-500')} />
          {connected ? 'live' : 'offline'}
        </Badge>
        {status && (
          <Badge variant="secondary" className="hidden sm:inline-flex">
            <Activity className="mr-1 h-3 w-3" />
            {status.watching} watching · {status.interval_sec}s
          </Badge>
        )}
        <ThemeToggle />
      </div>
    </header>
  )
}

export function StatCard({ title, value, hint, icon: Icon, className }) {
  return (
    <Card className={className}>
      <CardContent className="flex items-center gap-4 p-5">
        <div className="wobble-sm flex h-11 w-11 shrink-0 -rotate-3 items-center justify-center border-2 border-foreground/15 bg-accent/45">
          <Icon className="h-5 w-5 text-accent-foreground" strokeWidth={2.4} />
        </div>
        <div className="min-w-0">
          <p className="font-display text-xs font-bold uppercase tracking-wide text-muted-foreground">{title}</p>
          <p className="truncate font-display text-3xl font-extrabold leading-tight">{value}</p>
          {hint && <p className="truncate text-xs text-muted-foreground">{hint}</p>}
        </div>
      </CardContent>
    </Card>
  )
}
