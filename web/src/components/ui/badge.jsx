import * as React from 'react'
import { cva } from 'class-variance-authority'
import { cn } from '../../lib/utils.js'

const badgeVariants = cva(
  'wobble-sm inline-flex items-center gap-1 border-2 px-2.5 py-0.5 font-display text-xs font-bold tracking-wide transition-colors focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2',
  {
    variants: {
      variant: {
        default: 'border-foreground/20 bg-primary text-primary-foreground',
        secondary: 'border-foreground/15 bg-secondary text-secondary-foreground',
        destructive: 'border-foreground/20 bg-destructive text-destructive-foreground',
        outline: 'border-foreground/30 text-foreground',
        success: 'border-emerald-700/25 bg-emerald-400/30 text-emerald-800 dark:text-emerald-200',
        warning: 'border-amber-700/25 bg-amber-300/40 text-amber-800 dark:text-amber-200',
      },
    },
    defaultVariants: { variant: 'default' },
  },
)

function Badge({ className, variant, ...props }) {
  return <div className={cn(badgeVariants({ variant }), className)} {...props} />
}

export { Badge, badgeVariants }
