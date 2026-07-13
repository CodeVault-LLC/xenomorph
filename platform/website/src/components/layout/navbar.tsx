import { Link, useMatchRoute } from "@tanstack/react-router"
import { MonitorCheck, Moon, Sun } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import { useTheme } from "@/stores/theme-store"

export const Navbar = () => {
  const navItems = [
    { to: "/", label: "Clients", exact: true },
    { to: "/terms", label: "Glossary", exact: true },
  ] as const

  return (
    <div className="sticky top-0 z-30 border-b border-border bg-background/80 backdrop-blur supports-backdrop-filter:bg-background/60">
      <div className="mx-auto flex w-full max-w-7xl items-center justify-between gap-4 px-4 py-3 sm:px-6 lg:px-8">
        <Button
          render={<Link to="/" />}
          nativeButton={false}
          variant="ghost"
          className="justify-start px-1"
        >
          <span className="flex size-8 items-center justify-center rounded-lg border border-border bg-card shadow-sm">
            <MonitorCheck />
          </span>
          <span className="text-sm font-semibold tracking-normal">
            xenomorph
          </span>
        </Button>

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
    <Button
      render={<Link to={to} />}
      nativeButton={false}
      variant={isActive ? "secondary" : "ghost"}
      className={cn(!isActive && "text-muted-foreground")}
    >
      {label}
    </Button>
  )
}

const ThemeToggle = () => {
  const { theme, setTheme } = useTheme()
  const next = theme === "dark" ? "light" : "dark"

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="ghost"
            size="icon"
            aria-label={`Switch to ${next} mode`}
            onClick={() => setTheme(next)}
            className="ml-1"
          />
        }
      >
        {theme === "dark" ? <Sun /> : <Moon />}
      </TooltipTrigger>
      <TooltipContent>Switch to {next} mode</TooltipContent>
    </Tooltip>
  )
}
