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
    <Button variant="ghost" size="icon" onClick={() => setDark((d) => !d)} aria-label="Toggle theme">
      {dark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
    </Button>
  )
}

export function Sidebar({ active, onNavigate, counts }) {
  return (
    <aside className="hidden w-60 shrink-0 flex-col gap-1 border-r bg-muted/20 p-4 lg:flex">
      <div className="mb-4 flex items-center gap-2 px-2">
        <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary text-primary-foreground">
          <Zap className="h-5 w-5" />
        </div>
        <div>
          <p className="text-sm font-bold leading-none">pokebot-go</p>
          <p className="text-xs text-muted-foreground">restock monitor</p>
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
              'flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors',
              isActive ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
            )}
          >
            <Icon className="h-4 w-4 shrink-0" />
            <span className="flex-1 text-left">{n.label}</span>
            {count != null && count > 0 && (
              <span className={cn(
                'rounded-full px-2 py-0.5 text-xs font-semibold',
                isActive ? 'bg-primary-foreground/20' : 'bg-muted text-muted-foreground',
              )}>
                {count}
              </span>
            )}
          </button>
        )
      })}
      <div className="mt-auto rounded-md border bg-card p-3 text-xs text-muted-foreground">
        <p className="flex items-center gap-1.5 font-medium text-foreground">
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
    <div className="flex gap-1 overflow-x-auto pb-1 lg:hidden">
      {NAV.map((n) => {
        const Icon = n.icon
        return (
          <button
            key={n.id}
            onClick={() => onNavigate(n.id)}
            className={cn(
              'flex shrink-0 items-center gap-1.5 rounded-md px-3 py-2 text-sm font-medium',
              active === n.id ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground',
            )}
          >
            <Icon className="h-4 w-4" />
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
        <h1 className="truncate text-xl font-bold tracking-tight">Mission control</h1>
        <p className="text-sm text-muted-foreground">
          Monitor + scheduled drops + checkout profile — all local.
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
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-muted">
          <Icon className="h-5 w-5 text-muted-foreground" />
        </div>
        <div className="min-w-0">
          <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{title}</p>
          <p className="truncate text-2xl font-bold leading-tight">{value}</p>
          {hint && <p className="truncate text-xs text-muted-foreground">{hint}</p>}
        </div>
      </CardContent>
    </Card>
  )
}
