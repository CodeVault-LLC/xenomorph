import { Link, Outlet, useMatchRoute } from "@tanstack/react-router"
import {
  Activity,
  Cpu,
  FileStack,
  Monitor,
  ScrollText,
  Terminal,
} from "lucide-react"

import { OSLabel } from "@/components/dashboard/os-badge"
import { StatusBadge } from "@/components/dashboard/status-badge"
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Separator } from "@/components/ui/separator"
import { type ClientSnapshot, display, formatIP } from "@/lib/clients"

const tabs = [
  {
    id: "general",
    label: "General",
    icon: Activity,
    to: "/agents/$agentId" as const,
  },
  {
    id: "system",
    label: "System",
    icon: Cpu,
    to: "/agents/$agentId/system" as const,
  },
  {
    id: "files",
    label: "Files",
    icon: FileStack,
    to: "/agents/$agentId/files" as const,
  },
  {
    id: "terminal",
    label: "Terminal",
    icon: Terminal,
    to: "/agents/$agentId/terminal" as const,
  },
  {
    id: "screen",
    label: "Screen",
    icon: Monitor,
    to: "/agents/$agentId/screen" as const,
  },
  {
    id: "logs",
    label: "Logs",
    icon: ScrollText,
    to: "/agents/$agentId/logs" as const,
  },
] as const

export const AgentWorkspace = ({ client }: { client: ClientSnapshot }) => {
  return (
    <Tabs
      orientation="vertical"
      className="grid overflow-hidden rounded-lg border border-border bg-card md:h-[calc(100vh-190px)] md:grid-cols-[256px_minmax(0,1fr)]"
    >
      <WorkspaceSidebar client={client} />

      <div className="min-w-0 overflow-y-auto bg-background p-4 sm:p-5 lg:p-6">
        <Outlet />
      </div>
    </Tabs>
  )
}

function WorkspaceSidebar({ client }: { client: ClientSnapshot }) {
  const matchRoute = useMatchRoute()

  return (
    <Sidebar>
      <SidebarHeader>
        <div className="flex flex-wrap gap-2">
          <OSLabel value={client.os_version} />
        </div>
      </SidebarHeader>

      <SidebarContent className="overflow-hidden">
        <SidebarGroup>
          <SidebarGroupLabel>Workspace</SidebarGroupLabel>
          <TabsList variant="line" className="h-auto w-full items-stretch p-0">
            <SidebarMenu>
              {tabs.map((tab) => {
                const Icon = tab.icon
                const isActive = Boolean(
                  matchRoute({
                    to: tab.to,
                    params: { agentId: client.agent_id },
                    fuzzy: false,
                  })
                )
                return (
                  <SidebarMenuItem key={tab.id}>
                    <TabsTrigger
                      value={tab.id}
                      className="h-9 w-full justify-start px-2.5"
                      data-active={isActive}
                      render={
                        <Link
                          to={tab.to}
                          params={{ agentId: client.agent_id }}
                        />
                      }
                    >
                      <Icon data-icon="inline-start" />
                      {tab.label}
                    </TabsTrigger>
                  </SidebarMenuItem>
                )
              })}
            </SidebarMenu>
          </TabsList>
        </SidebarGroup>

        <SidebarGroup className="mt-auto pt-4">
          <Separator />

          <div className="grid gap-3 px-2 text-xs">
            <div className="mt-4 flex w-full min-w-0 flex-row items-center justify-between gap-2 overflow-hidden">
              <h2 className="truncate text-lg font-semibold">
                {display(client.hostname)}
              </h2>

              <StatusBadge online={client.is_online} />
            </div>
            <span className="truncate text-muted-foreground">
              {formatIP(client.client_ip)}
            </span>
          </div>
        </SidebarGroup>
      </SidebarContent>
    </Sidebar>
  )
}
