import { Link } from "@tanstack/react-router"

import { glossaryTerm } from "@/lib/glossary"
import { cn } from "@/lib/utils"

type TermLinkProps = {
  slug: string
  className?: string
  children?: React.ReactNode
}

export function TermLink({ slug, className, children }: TermLinkProps) {
  const term = glossaryTerm(slug)
  const label = children ?? term?.term ?? slug

  return (
    <Link
      to="/terms"
      hash={slug}
      className={cn(
        "font-medium text-foreground underline decoration-muted-foreground decoration-dotted underline-offset-4 outline-none hover:decoration-foreground focus-visible:decoration-foreground",
        className
      )}
    >
      {label}
    </Link>
  )
}
