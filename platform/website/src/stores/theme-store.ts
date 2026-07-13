import { create } from "zustand"

export type Theme = "dark" | "light" | "system"

type ThemeStore = {
  theme: Theme
  setTheme: (theme: Theme) => void
}

export const useThemeStore = create<ThemeStore>()((set) => ({
  theme: "dark",
  setTheme: (theme) => set({ theme }),
}))

export function useTheme() {
  const theme = useThemeStore((state) => state.theme)
  const setTheme = useThemeStore((state) => state.setTheme)

  return { theme, setTheme }
}
