import { StrictMode } from "react"
import { createRoot } from "react-dom/client"

import "./index.css"
import { ClientsStream } from "@/components/data/clients-query"
import { ThemeProvider } from "@/components/theme-provider.tsx"
import { TooltipProvider } from "@/components/ui/tooltip"
import { QueryClientProvider } from "@tanstack/react-query"
import { createRouter, RouterProvider } from "@tanstack/react-router"
import { queryClient } from "@/lib/query-client"

// Import the generated route tree
import { routeTree } from "./routeTree.gen"
import { ErrorBanner } from "./components/layout/error-banner"

// Create a new router instance
const router = createRouter({
  routeTree,
  context: { queryClient },
  defaultErrorComponent: (err) => (
    <ErrorBanner message={err.error.message}></ErrorBanner>
  ),
})

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router
  }
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ClientsStream />
      <ThemeProvider defaultTheme="dark">
        <TooltipProvider>
          <RouterProvider router={router} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  </StrictMode>
)
