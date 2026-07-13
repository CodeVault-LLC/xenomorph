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

import { Alert, AlertDescription } from "@/components/ui/alert"
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
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { Field, FieldDescription, FieldGroup } from "@/components/ui/field"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupTextarea,
  InputGroupText,
} from "@/components/ui/input-group"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
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
  const inputRef = React.useRef<HTMLTextAreaElement | null>(null)

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

  function handleCommandKeyDown(
    event: React.KeyboardEvent<HTMLTextAreaElement>
  ) {
    if (
      event.key !== "Enter" ||
      event.shiftKey ||
      event.nativeEvent.isComposing
    ) {
      return
    }

    event.preventDefault()
    event.currentTarget.form?.requestSubmit()
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
    <Card className="flex h-[min(640px,calc(100dvh-220px))] min-h-80 flex-col overflow-hidden md:h-full">
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
      <CardContent className="grid min-h-0 flex-1 grid-rows-[auto_auto_auto_auto_minmax(0,1fr)_auto] overflow-hidden p-0">
        <div className="flex min-w-0 items-center gap-2 bg-muted/30 px-3 py-2">
          <Tabs
            value={activeSessionID || undefined}
            onValueChange={(value) => {
              setActiveSessionID(value as string)
              focusInput()
            }}
            className="min-w-0 flex-1"
          >
            <TabsList className="max-w-full justify-start overflow-x-auto bg-transparent p-0">
              {loading ? <Skeleton className="h-8 w-28" /> : null}
              {!loading && sessions.length === 0 ? (
                <span className="text-sm text-muted-foreground">
                  No sessions
                </span>
              ) : null}
              {sessions.map((session, index) => (
                <TabsTrigger
                  key={session.session_id}
                  value={session.session_id}
                  className="shrink-0"
                >
                  <Shell data-icon="inline-start" />
                  {session.label || `Terminal ${index + 1}`}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
          <NewTerminalMenu
            shells={shellChoices}
            selectedShell={selectedShell}
            onSelectShell={handleNewTerminal}
          />
        </div>
        <Separator />

        <div className="flex items-center gap-2 px-3 py-2 text-sm text-muted-foreground">
          <Shell className="size-4" />
          <span>{displayedShell}</span>
          {activeSession ? (
            <TerminalOptions
              canCopyHistory={historyText.length > 0}
              onCopyHistory={copyHistory}
              onCopySessionID={copySessionID}
              onDeleteSession={handleDeleteSession}
            />
          ) : null}
        </div>
        <Separator />

        <ScrollArea
          ref={outputRef}
          className="min-h-0 overflow-auto bg-zinc-950 p-3 font-mono text-[13px] leading-6 text-zinc-100"
        >
          {error ? (
            <Alert variant="destructive" className="mb-3">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}

          {!activeSession && entries.length === 0 ? (
            <TerminalEmpty
              title="No active session"
              description="Type a command to start a new terminal session."
            />
          ) : null}

          {activeSession && entries.length === 0 ? (
            <TerminalEmpty
              title="No commands yet"
              description="Commands submitted through this session will appear here."
            />
          ) : null}

          {entries.map((entry) => (
            <TerminalEntryBlock key={entry.command_id} entry={entry} />
          ))}
        </ScrollArea>

        <form onSubmit={handleSubmit} className="border-t bg-background p-3">
          <FieldGroup className="gap-0">
            <Field
              className="gap-0"
              data-disabled={!client.is_online || undefined}
            >
              <InputGroup className="bg-muted/30">
                <InputGroupTextarea
                  ref={inputRef}
                  value={command}
                  onChange={(event) => setCommand(event.target.value)}
                  onKeyDown={handleCommandKeyDown}
                  disabled={!client.is_online}
                  placeholder={
                    client.is_online
                      ? "Enter a shell command"
                      : "Agent is offline"
                  }
                  rows={2}
                  aria-label="Terminal command"
                  className="min-h-20 font-mono"
                />
                <InputGroupAddon align="block-start" className="border-b">
                  <InputGroupText className="font-mono text-foreground">
                    {promptLabel(activeSession)}
                    <span className="font-sans text-muted-foreground">
                      Command
                    </span>
                  </InputGroupText>
                </InputGroupAddon>
                <InputGroupAddon
                  align="block-end"
                  className="justify-between border-t"
                >
                  <FieldDescription className="text-xs">
                    Enter to run · Shift+Enter for a new line
                  </FieldDescription>
                  <Button
                    type="submit"
                    size="sm"
                    disabled={
                      !client.is_online || submitting || !command.trim()
                    }
                  >
                    {submitting ? (
                      <Spinner data-icon="inline-start" />
                    ) : (
                      <Play data-icon="inline-start" />
                    )}
                    {submitting ? "Running" : "Run command"}
                  </Button>
                </InputGroupAddon>
              </InputGroup>
            </Field>
          </FieldGroup>
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
        render={<Button variant="outline" size="icon" />}
        aria-label="New terminal session"
      >
        <Plus data-icon="inline-start" />
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
                <Shell />
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
        render={<Button variant="outline" size="icon" className="ml-auto" />}
        aria-label="Terminal session options"
      >
        <MoreHorizontal data-icon="inline-start" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-48">
        <DropdownMenuGroup>
          <DropdownMenuLabel>Session</DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={onCopyHistory} disabled={!canCopyHistory}>
            <Copy />
            Copy history
          </DropdownMenuItem>
          <DropdownMenuItem onClick={onCopySessionID}>
            <Copy />
            Copy session ID
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={onDeleteSession} variant="destructive">
            <Trash2 />
            Delete session
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function TerminalEmpty({
  title,
  description,
}: {
  title: string
  description: string
}) {
  return (
    <Empty className="min-h-52 border border-dashed border-zinc-800 bg-zinc-950 text-zinc-100">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <Terminal />
        </EmptyMedia>
        <EmptyTitle>{title}</EmptyTitle>
        <EmptyDescription>{description}</EmptyDescription>
      </EmptyHeader>
    </Empty>
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
