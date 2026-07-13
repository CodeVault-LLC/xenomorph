import { Outlet, createRootRoute } from "@tanstack/react-router"
import * as React from "react"

import { Navbar } from "@/components/layout/navbar"

export const Route = createRootRoute({
  component: RootComponent,
})

function RootComponent() {
  return (
    <React.Fragment>
      <Navbar />
      <Outlet />
    </React.Fragment>
  )
}
