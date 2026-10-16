import { cn } from '../../lib/utils.js'

function Skeleton({ className, ...props }) {
  return <div className={cn('wobble-sm animate-pulse bg-muted', className)} {...props} />
}

export { Skeleton }
