import * as React from "react"
import { type Theme, useThemeStore } from "@/stores/theme-store"

type ResolvedTheme = "dark" | "light"

type ThemeProviderProps = {
  children: React.ReactNode
  defaultTheme?: Theme
  storageKey?: string
  disableTransitionOnChange?: boolean
}

const COLOR_SCHEME_QUERY = "(prefers-color-scheme: dark)"
const THEME_VALUES: readonly Theme[] = ["dark", "light", "system"]

function isTheme(value: string | null): value is Theme {
  return value !== null && THEME_VALUES.includes(value as Theme)
}

function getSystemTheme(): ResolvedTheme {
  return window.matchMedia(COLOR_SCHEME_QUERY).matches ? "dark" : "light"
}

function disableTransitionsTemporarily() {
  const style = document.createElement("style")
  style.appendChild(
    document.createTextNode(
      "*,*::before,*::after{-webkit-transition:none!important;transition:none!important}"
    )
  )
  document.head.appendChild(style)

  return () => {
    window.getComputedStyle(document.body)
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        style.remove()
      })
    })
  }
}

function isEditableTarget(target: EventTarget | null) {
  return (
    target instanceof HTMLElement &&
    (target.isContentEditable ||
      target.closest("input, textarea, select, [contenteditable='true']") !==
        null)
  )
}

/** Applies the persisted UI preference; it does not own remote application data. */
export function ThemeProvider({
  children,
  defaultTheme = "system",
  storageKey = "theme",
  disableTransitionOnChange = true,
}: ThemeProviderProps) {
  const theme = useThemeStore((state) => state.theme)
  const setTheme = useThemeStore((state) => state.setTheme)

  React.useEffect(() => {
    const storedTheme = localStorage.getItem(storageKey)
    setTheme(isTheme(storedTheme) ? storedTheme : defaultTheme)
  }, [defaultTheme, setTheme, storageKey])

  React.useEffect(() => {
    const resolvedTheme = theme === "system" ? getSystemTheme() : theme
    const restoreTransitions = disableTransitionOnChange
      ? disableTransitionsTemporarily()
      : undefined

    document.documentElement.classList.remove("light", "dark")
    document.documentElement.classList.add(resolvedTheme)
    localStorage.setItem(storageKey, theme)
    restoreTransitions?.()
  }, [disableTransitionOnChange, storageKey, theme])

  React.useEffect(() => {
    if (theme !== "system") {
      return undefined
    }

    const mediaQuery = window.matchMedia(COLOR_SCHEME_QUERY)
    const handleChange = () => {
      document.documentElement.classList.toggle(
        "dark",
        getSystemTheme() === "dark"
      )
      document.documentElement.classList.toggle(
        "light",
        getSystemTheme() === "light"
      )
    }
    mediaQuery.addEventListener("change", handleChange)

    return () => mediaQuery.removeEventListener("change", handleChange)
  }, [theme])

  React.useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (
        event.repeat ||
        event.metaKey ||
        event.ctrlKey ||
        event.altKey ||
        isEditableTarget(event.target) ||
        event.key.toLowerCase() !== "d"
      ) {
        return
      }

      setTheme(theme === "dark" ? "light" : "dark")
    }
    window.addEventListener("keydown", handleKeyDown)

    return () => window.removeEventListener("keydown", handleKeyDown)
  }, [setTheme, theme])

  React.useEffect(() => {
    const handleStorageChange = (event: StorageEvent) => {
      if (event.storageArea === localStorage && event.key === storageKey) {
        setTheme(isTheme(event.newValue) ? event.newValue : defaultTheme)
      }
    }
    window.addEventListener("storage", handleStorageChange)

    return () => window.removeEventListener("storage", handleStorageChange)
  }, [defaultTheme, setTheme, storageKey])

  return children
}
