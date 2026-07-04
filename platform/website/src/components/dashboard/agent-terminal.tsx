import * as React from "react"
import {
  Circle,
  Copy,
  MoreHorizontal,
  Play,
  Plus,
  RefreshCw,
  Shell,
  Terminal,
  Trash2,
} from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { type ClientSnapshot, formatDate } from "@/lib/clients"
import {
  createTerminalSession,
  deleteTerminalSession,
  enqueueTerminalCommand,
  fetchTerminalEntries,
  fetchTerminalSessions,
  type TerminalEntry,
  type TerminalSession,
} from "@/lib/terminal"

export function AgentTerminal({ client }: { client: ClientSnapshot }) {
  const [sessions, setSessions] = React.useState<TerminalSession[]>([])
  const [activeSessionID, setActiveSessionID] = React.useState("")
  const [entries, setEntries] = React.useState<TerminalEntry[]>([])
  const [command, setCommand] = React.useState("")
  const [selectedShell, setSelectedShell] = React.useState(
    defaultShell(client.os_version)
  )
  const [loading, setLoading] = React.useState(true)
  const [submitting, setSubmitting] = React.useState(false)
  const [error, setError] = React.useState<string | null>(null)
  const outputRef = React.useRef<HTMLDivElement | null>(null)
  const inputRef = React.useRef<HTMLInputElement | null>(null)

  const activeSession = sessions.find(
    (session) => session.session_id === activeSessionID
  )
  const shellChoices = terminalShellChoices(client.os_version)
  const displayedShell = activeSession?.shell || selectedShell
  const historyText = entries
    .map((entry) => {
      const output = entry.output_log || entry.reason
      return output ? `$ ${entry.command}\n${output}` : `$ ${entry.command}`
    })
    .join("\n")

  const focusInput = React.useCallback(() => {
    window.setTimeout(() => inputRef.current?.focus(), 0)
  }, [])

  const refreshSessions = React.useCallback(() => {
    setLoading(true)
    setError(null)
    fetchTerminalSessions(client.agent_id)
      .then((snapshot) => {
        setSessions(snapshot)
        setActiveSessionID((current) => {
          if (!current) {
            return snapshot[0]?.session_id || ""
          }
          return snapshot.some((session) => session.session_id === current)
            ? current
            : snapshot[0]?.session_id || ""
        })
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : "Terminal unavailable")
      })
      .finally(() => {
        setLoading(false)
      })
  }, [client.agent_id])

  const refreshEntries = React.useCallback(() => {
    if (!activeSessionID) {
      setEntries([])
      return
    }
    fetchTerminalEntries(client.agent_id, activeSessionID)
      .then(setEntries)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : "Terminal entries failed")
      })
  }, [activeSessionID, client.agent_id])

  React.useEffect(() => {
    const initial = window.setTimeout(refreshSessions, 0)
    return () => window.clearTimeout(initial)
  }, [refreshSessions])

  React.useEffect(() => {
    const initial = window.setTimeout(refreshEntries, 0)
    const interval = window.setInterval(refreshEntries, 1500)
    return () => {
      window.clearTimeout(initial)
      window.clearInterval(interval)
    }
  }, [refreshEntries])

  React.useEffect(() => {
    outputRef.current?.scrollTo({
      top: outputRef.current.scrollHeight,
      behavior: "smooth",
    })
  }, [entries])

  async function ensureSession() {
    if (activeSession) {
      return activeSession
    }

    const session = await createTerminalSession(client.agent_id, {
      label: `Terminal ${sessions.length + 1}`,
      shell: selectedShell,
    })
    setSessions((current) => [session, ...current])
    setActiveSessionID(session.session_id)
    return session
  }

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const nextCommand = command.trim()
    if (!nextCommand || !client.is_online) {
      focusInput()
      return
    }

    setCommand("")
    setSubmitting(true)
    setError(null)
    try {
      const session = await ensureSession()
      const queued = await enqueueTerminalCommand(
        client.agent_id,
        session.session_id,
        { command: nextCommand }
      )
      if (queued) {
        setEntries((current) => [...current, queued])
      }
      window.setTimeout(() => {
        fetchTerminalEntries(client.agent_id, session.session_id)
          .then(setEntries)
          .catch((err: unknown) => {
            setError(
              err instanceof Error ? err.message : "Terminal entries failed"
            )
          })
      }, 500)
    } catch (err) {
      setCommand(nextCommand)
      setError(err instanceof Error ? err.message : "Terminal command failed")
    } finally {
      setSubmitting(false)
      focusInput()
    }
  }

  function handleNewTerminal(shell = selectedShell) {
    setSelectedShell(shell)
    setActiveSessionID("")
    setEntries([])
    setError(null)
    focusInput()
  }

  async function handleDeleteSession() {
    if (!activeSession) {
      return
    }

    setError(null)
    try {
      await deleteTerminalSession(client.agent_id, activeSession.session_id)
      const remaining = sessions.filter(
        (session) => session.session_id !== activeSession.session_id
      )
      setSessions(remaining)
      setActiveSessionID(remaining[0]?.session_id || "")
      if (remaining.length === 0) {
        setEntries([])
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Terminal delete failed")
    } finally {
      focusInput()
    }
  }

  async function copyHistory() {
    await navigator.clipboard.writeText(historyText)
    focusInput()
  }

  async function copySessionID() {
    if (activeSession) {
      await navigator.clipboard.writeText(activeSession.session_id)
    }
    focusInput()
  }

  return (
    <Card className="overflow-visible">
      <CardHeader className="border-b">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle className="flex items-center gap-2">
              <Terminal className="size-4 text-muted-foreground" />
              Terminal
            </CardTitle>
            <CardDescription>
              Gateway-authorized shell access for this authenticated agent.
            </CardDescription>
          </div>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              refreshSessions()
              refreshEntries()
              focusInput()
            }}
          >
            <RefreshCw />
            Refresh
          </Button>
        </div>
      </CardHeader>
      <CardContent className="grid h-[min(720px,calc(100vh-260px))] min-h-[520px] grid-rows-[auto_auto_minmax(0,1fr)_auto] overflow-hidden p-0">
        <div className="flex min-w-0 items-center gap-2 border-b bg-muted/30 px-3 py-2">
          <div className="flex min-w-0 flex-1 items-center gap-2 overflow-x-auto">
            {loading ? (
              <span className="text-sm text-muted-foreground">Loading</span>
            ) : null}
            {!loading && sessions.length === 0 ? (
              <span className="text-sm text-muted-foreground">No sessions</span>
            ) : null}
            {sessions.map((session, index) => (
              <button
                key={session.session_id}
                type="button"
                onClick={() => {
                  setActiveSessionID(session.session_id)
                  focusInput()
                }}
                className={`inline-flex h-9 shrink-0 items-center gap-2 rounded-md border px-3 text-sm font-medium transition-colors ${
                  activeSessionID === session.session_id
                    ? "border-primary bg-background shadow-sm"
                    : "border-transparent bg-transparent text-muted-foreground hover:border-border hover:bg-background/80 hover:text-foreground"
                }`}
              >
                <Shell className="size-4" />
                {session.label || `Terminal ${index + 1}`}
              </button>
            ))}
          </div>
          <NewTerminalMenu
            shells={shellChoices}
            selectedShell={selectedShell}
            onSelectShell={handleNewTerminal}
          />
        </div>

        <div className="flex flex-wrap items-center gap-2 border-b px-3 py-2">
          <Badge variant={client.is_online ? "online" : "offline"}>
            {client.is_online ? "Online" : "Offline"}
          </Badge>
          <Badge variant="outline">{displayedShell}</Badge>
          {activeSession ? (
            <TerminalOptions
              canCopyHistory={historyText.length > 0}
              onCopyHistory={copyHistory}
              onCopySessionID={copySessionID}
              onDeleteSession={handleDeleteSession}
            />
          ) : null}
        </div>

        <div
          ref={outputRef}
          className="min-h-0 overflow-auto bg-zinc-950 p-3 font-mono text-[13px] leading-6 text-zinc-100"
          onClick={focusInput}
        >
          {error ? (
            <div className="mb-3 rounded border border-red-400/40 bg-red-950/40 px-3 py-2 text-red-100">
              {error}
            </div>
          ) : null}

          {!activeSession && entries.length === 0 ? (
            <div className="text-zinc-400">
              Type a command to start a new terminal session.
            </div>
          ) : null}

          {activeSession && entries.length === 0 ? (
            <div className="text-zinc-400">No commands have run here.</div>
          ) : null}

          {entries.map((entry) => (
            <TerminalEntryBlock key={entry.command_id} entry={entry} />
          ))}
        </div>

        <form onSubmit={handleSubmit} className="border-t bg-background p-3">
          <div className="flex min-w-0 items-center gap-2 rounded-md border bg-muted/30 px-2">
            <NewTerminalMenu
              shells={shellChoices}
              selectedShell={selectedShell}
              onSelectShell={handleNewTerminal}
            />
            <span className="font-mono text-sm text-muted-foreground">
              {promptLabel(activeSession)}
            </span>
            <input
              ref={inputRef}
              value={command}
              onChange={(event) => setCommand(event.target.value)}
              disabled={!client.is_online}
              placeholder={
                client.is_online ? "Run a shell command" : "Agent is offline"
              }
              className="h-10 min-w-0 flex-1 bg-transparent font-mono text-sm outline-none"
            />
            <Button
              type="submit"
              size="icon-sm"
              disabled={!client.is_online || submitting || !command.trim()}
              aria-label="Run command"
            >
              <Play />
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}

function NewTerminalMenu({
  shells,
  selectedShell,
  onSelectShell,
}: {
  shells: string[]
  selectedShell: string
  onSelectShell: (shell: string) => void
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md border bg-background text-foreground shadow-sm transition-colors hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50"
        aria-label="New terminal session"
      >
        <Plus className="size-4" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-48">
        <DropdownMenuGroup>
          <DropdownMenuLabel>New Terminal</DropdownMenuLabel>
          <DropdownMenuSeparator />
          {shells.map((shell) => (
            <DropdownMenuItem
              key={shell}
              onClick={() => onSelectShell(shell)}
              className="justify-between"
            >
              <span className="inline-flex items-center gap-2">
                <Shell className="size-4" />
                {shell}
              </span>
              {selectedShell === shell ? (
                <span className="text-xs text-muted-foreground">Current</span>
              ) : null}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function TerminalOptions({
  canCopyHistory,
  onCopyHistory,
  onCopySessionID,
  onDeleteSession,
}: {
  canCopyHistory: boolean
  onCopyHistory: () => void
  onCopySessionID: () => void
  onDeleteSession: () => void
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className="ml-auto inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md border bg-background transition-colors hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50"
        aria-label="Terminal session options"
      >
        <MoreHorizontal className="size-4" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-48">
        <DropdownMenuGroup>
          <DropdownMenuLabel>Session</DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={onCopyHistory} disabled={!canCopyHistory}>
            <Copy className="size-4" />
            Copy history
          </DropdownMenuItem>
          <DropdownMenuItem onClick={onCopySessionID}>
            <Copy className="size-4" />
            Copy session ID
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={onDeleteSession} variant="destructive">
            <Trash2 className="size-4" />
            Delete session
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function TerminalEntryBlock({ entry }: { entry: TerminalEntry }) {
  return (
    <div className="mb-4 grid gap-1">
      <div className="flex flex-wrap items-center gap-2 text-zinc-400">
        <Circle
          className={`size-2.5 fill-current ${
            entry.status === "queued"
              ? "text-amber-300"
              : entry.exit_code === 0
                ? "text-emerald-300"
                : "text-red-300"
          }`}
        />
        <span>{entry.shell}</span>
        <span>{formatDate(entry.submitted_at)}</span>
      </div>
      <div className="break-words text-cyan-100">$ {entry.command}</div>
      {entry.status === "queued" ? (
        <div className="text-amber-200">queued</div>
      ) : (
        <pre className="max-w-full break-words whitespace-pre-wrap text-zinc-100">
          {entry.output_log || entry.reason || `exit ${entry.exit_code}`}
        </pre>
      )}
    </div>
  )
}

function defaultShell(osVersion: string) {
  const normalized = osVersion.toLowerCase()
  if (normalized.includes("windows")) {
    return "powershell"
  }
  if (normalized.includes("darwin") || normalized.includes("mac")) {
    return "zsh"
  }
  return "bash"
}

function terminalShellChoices(osVersion: string) {
  const normalized = osVersion.toLowerCase()
  if (normalized.includes("windows")) {
    return ["powershell", "pwsh", "cmd"]
  }
  if (normalized.includes("darwin") || normalized.includes("mac")) {
    return ["zsh", "bash", "sh"]
  }
  return ["bash", "zsh", "sh", "pwsh"]
}

function promptLabel(session?: TerminalSession) {
  if (!session) {
    return "$"
  }
  if (session.shell === "powershell" || session.shell === "pwsh") {
    return "PS>"
  }
  if (session.shell === "cmd") {
    return ">"
  }
  return "$"
}
