import * as React from 'react'
import { cn } from '../../lib/utils.js'

function Table({ className, ...props }) {
  return (
    <div className="relative w-full overflow-auto">
      <table className={cn('w-full caption-bottom text-sm', className)} {...props} />
    </div>
  )
}

function TableHeader({ className, ...props }) {
  return <thead className={cn('[&_tr]:border-b-2 [&_tr]:border-border', className)} {...props} />
}

function TableBody({ className, ...props }) {
  return <tbody className={cn('[&_tr:last-child]:border-0', className)} {...props} />
}

function TableRow({ className, ...props }) {
  return (
    <tr
      className={cn(
        'border-b-2 border-border/50 transition-[background-color] duration-150 hover:bg-accent/25 data-[state=selected]:bg-muted',
        className,
      )}
      {...props}
    />
  )
}

function TableHead({ className, ...props }) {
  return (
    <th
      className={cn(
        'h-11 px-4 text-left align-middle font-display text-xs font-bold uppercase tracking-wide text-muted-foreground [&:has([role=checkbox])]:pr-0',
        className,
      )}
      {...props}
    />
  )
}

function TableCell({ className, ...props }) {
  return <td className={cn('p-4 align-middle [&:has([role=checkbox])]:pr-0', className)} {...props} />
}

export { Table, TableHeader, TableBody, TableHead, TableRow, TableCell }
