export type TerminalSession = {
  agent_id: string
  session_id: string
  label: string
  shell: string
  working_directory: string
  created_at: string
  updated_at: string
  last_command_id: string
}

export type TerminalEntry = {
  agent_id: string
  session_id: string
  command_id: string
  command: string
  shell: string
  working_directory: string
  status: "queued" | "executed" | "rejected" | string
  exit_code: number
  output_log: string
  reason: string
  submitted_at: string
  completed_at?: string
}

type SessionsResponse = {
  sessions?: TerminalSession[]
  session?: TerminalSession
}

type EntriesResponse = {
  entries?: TerminalEntry[]
  entry?: TerminalEntry
}

export async function fetchTerminalSessions(agentId: string) {
  const response = await fetch(`/api/clients/${agentId}/terminal/sessions`, {
    cache: "no-store",
  })
  if (!response.ok) {
    throw new Error(`Terminal sessions API returned HTTP ${response.status}`)
  }

  const payload = (await response.json()) as SessionsResponse
  return Array.isArray(payload.sessions) ? payload.sessions : []
}

export async function createTerminalSession(
  agentId: string,
  input: { label?: string; shell?: string; working_directory?: string } = {}
) {
  const response = await fetch(`/api/clients/${agentId}/terminal/sessions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  })
  if (!response.ok) {
    throw new Error(`Terminal session create returned HTTP ${response.status}`)
  }

  const payload = (await response.json()) as SessionsResponse
  if (!payload.session) {
    throw new Error("Terminal session create returned no session")
  }
  return payload.session
}

export async function deleteTerminalSession(
  agentId: string,
  sessionId: string
) {
  const response = await fetch(
    `/api/clients/${agentId}/terminal/sessions/${sessionId}`,
    { method: "DELETE" }
  )
  if (!response.ok) {
    throw new Error(`Terminal session delete returned HTTP ${response.status}`)
  }
}

export async function fetchTerminalEntries(agentId: string, sessionId: string) {
  const response = await fetch(
    `/api/clients/${agentId}/terminal/sessions/${sessionId}/entries`,
    { cache: "no-store" }
  )
  if (!response.ok) {
    throw new Error(`Terminal entries API returned HTTP ${response.status}`)
  }

  const payload = (await response.json()) as EntriesResponse
  return Array.isArray(payload.entries) ? payload.entries : []
}

export async function enqueueTerminalCommand(
  agentId: string,
  sessionId: string,
  input: { command: string; working_directory?: string }
) {
  const response = await fetch(
    `/api/clients/${agentId}/terminal/sessions/${sessionId}/commands`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    }
  )
  if (!response.ok) {
    throw new Error(`Terminal command API returned HTTP ${response.status}`)
  }

  const payload = (await response.json()) as EntriesResponse
  return payload.entry
}
