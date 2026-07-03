import { Outlet, createRootRoute } from "@tanstack/react-router"
import * as React from "react"

import { SiteChrome } from "@/components/layout/site-chrome"

export const Route = createRootRoute({
  component: RootComponent,
})

function RootComponent() {
  return (
    <React.Fragment>
      <SiteChrome />
      <Outlet />
    </React.Fragment>
  )
}
