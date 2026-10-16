import * as React from 'react'
import { Slot } from '@radix-ui/react-slot'
import { cva } from 'class-variance-authority'
import { cn } from '../../lib/utils.js'

const buttonVariants = cva(
  'wobble-sm inline-flex items-center justify-center gap-2 whitespace-nowrap font-display text-sm font-bold tracking-wide ring-offset-background transition-[transform,filter,background-color] duration-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 active:translate-y-[1px] disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        default: 'doodle-shadow bg-primary text-primary-foreground hover:brightness-105',
        destructive: 'doodle-shadow bg-destructive text-destructive-foreground hover:brightness-105',
        outline: 'border-2 border-foreground/25 bg-card hover:bg-accent hover:text-accent-foreground',
        secondary: 'border-2 border-foreground/10 bg-secondary text-secondary-foreground hover:brightness-105',
        ghost: 'hover:bg-accent/60 hover:text-accent-foreground',
        link: 'text-primary underline-offset-4 hover:underline',
      },
      size: {
        default: 'h-10 px-5 py-2',
        sm: 'h-9 gap-1.5 px-3',
        lg: 'h-12 px-8',
        icon: 'h-10 w-10',
      },
    },
    defaultVariants: { variant: 'default', size: 'default' },
  },
)

const Button = React.forwardRef(({ className, variant, size, asChild = false, ...props }, ref) => {
  const Comp = asChild ? Slot : 'button'
  return <Comp className={cn(buttonVariants({ variant, size, className }))} ref={ref} {...props} />
})
Button.displayName = 'Button'

export { Button, buttonVariants }
