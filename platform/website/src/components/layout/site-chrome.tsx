import { Link, useMatchRoute } from "@tanstack/react-router"
import { MonitorCheck, Moon, Sun } from "lucide-react"

import { useTheme } from "@/components/theme-provider"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

const navItems = [
  { to: "/", label: "Clients", exact: true },
  { to: "/terms", label: "Glossary", exact: true },
] as const

export function SiteChrome() {
  return (
    <div className="sticky top-0 z-30 border-b border-border bg-background/80 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="mx-auto flex w-full max-w-7xl items-center justify-between gap-4 px-4 py-3 sm:px-6 lg:px-8">
        <Link
          to="/"
          className="inline-flex shrink-0 items-center gap-2 outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
        >
          <span className="flex size-8 items-center justify-center rounded-lg border border-border bg-card shadow-sm">
            <MonitorCheck className="size-5 text-foreground" />
          </span>
          <span className="text-sm font-semibold tracking-normal">
            xenomorph
          </span>
        </Link>

        <nav className="flex items-center gap-1">
          {navItems.map((item) => (
            <NavLink key={item.to} {...item} />
          ))}
          <ThemeToggle />
        </nav>
      </div>
    </div>
  )
}

function NavLink({
  to,
  label,
  exact,
}: {
  to: "/" | "/terms"
  label: string
  exact: boolean
}) {
  const matchRoute = useMatchRoute()
  const isActive = Boolean(matchRoute({ to, fuzzy: !exact }))
  return (
    <Link
      to={to}
      className={cn(
        "inline-flex h-8 shrink-0 items-center justify-center rounded-lg px-3 text-sm font-medium transition-all outline-none hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50",
        isActive ? "bg-muted text-foreground" : "text-muted-foreground"
      )}
    >
      {label}
    </Link>
  )
}

function ThemeToggle() {
  const { theme, setTheme } = useTheme()
  const next = theme === "dark" ? "light" : "dark"
  return (
    <Button
      variant="ghost"
      size="icon"
      aria-label={`Switch to ${next} mode`}
      onClick={() => setTheme(next)}
      className="ml-1"
    >
      {theme === "dark" ? <Sun /> : <Moon />}
    </Button>
  )
}
