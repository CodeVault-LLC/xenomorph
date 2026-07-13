export type FileVerb = "list" | "metadata" | "preview"

export interface FileRoot {
  root_id: string
  display_label: string
  allowed_verbs: FileVerb[]
}

export interface RootObservation {
  root_id: string
  display_label: string
  allowed_verbs: FileVerb[]
  available: boolean
  read_only: boolean
  error_class?: string
  capabilities: {
    operating_system: string
    no_follow_resolution: "available" | "unavailable" | "denied"
    safe_handle_relative_io: "available" | "unavailable" | "denied"
  }
}

interface RootsListResult {
  protocol_version: number
  roots: RootObservation[]
}

export type FileEntryKind = "file" | "directory" | "symlink" | "special"

export interface FileEntry {
  entry_id: string
  display_name: string
  operation_name?: string
  kind: FileEntryKind
  size: number
  modified_at: string
  mode: number
  hidden: boolean
  readable: boolean
}

export interface DirectoryPage {
  protocol_version: number
  root_id: string
  relative_path: string
  snapshot_id: string
  ordering: string
  entries: FileEntry[]
  next_cursor?: string
  has_more: boolean
}

export interface MetadataResult {
  protocol_version: number
  root_id: string
  relative_path: string
  kind: FileEntryKind
  size: number
  modified_at: string
  mode: number
  optional_fields: Record<
    string,
    { state: "available" | "unavailable" | "denied"; value?: string }
  >
}

export interface PreviewResult {
  protocol_version: number
  root_id: string
  relative_path: string
  offset: number
  data: string
  classification: "text" | "binary"
  truncated: boolean
}

interface FileCommandResult<T> {
  protocol_version: number
  data?: T
  error?: { class: string; retryable: boolean; message: string }
}

interface FileOperation<T> {
  operation_id: string
  command_id: string
  agent_id: string
  root_id: string
  type: string
  state: "queued" | "completed" | "failed"
  result?: FileCommandResult<T>
  error_class?: string
}

const jsonHeaders = {
  "Content-Type": "application/json",
}

const readJSON = async <T>(response: Response): Promise<T> => {
  const payload = (await response.json()) as T & { error?: string }
  if (!response.ok) {
    throw new Error(
      payload.error ?? `File API request failed (${response.status})`
    )
  }
  return payload
}

export const probeFileRoots = async (agentID: string, signal?: AbortSignal) =>
  createOperation<RootsListResult>(agentID, "roots/probe", undefined, signal)

const createOperation = async <T>(
  agentID: string,
  endpoint: string,
  body: unknown,
  signal?: AbortSignal
): Promise<T> => {
  const response = await fetch(`/api/clients/${agentID}/files/${endpoint}`, {
    method: "POST",
    headers: jsonHeaders,
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
  })
  const created = await readJSON<{ operation: FileOperation<T> }>(response)
  return pollOperation<T>(agentID, created.operation.operation_id, signal)
}

const pollOperation = async <T>(
  agentID: string,
  operationID: string,
  signal?: AbortSignal
): Promise<T> => {
  for (;;) {
    const response = await fetch(
      `/api/clients/${agentID}/files/operations/${operationID}`,
      { cache: "no-store", signal }
    )
    const payload = await readJSON<{ operation: FileOperation<T> }>(response)
    const operation = payload.operation
    if (operation.state === "failed") {
      throw new Error(operation.error_class ?? "File operation failed")
    }
    if (operation.state === "completed") {
      if (operation.result?.error) {
        throw new Error(operation.result.error.message)
      }
      if (operation.result?.data === undefined) {
        throw new Error("File operation returned no result")
      }
      return operation.result.data
    }
    await abortableDelay(300, signal)
  }
}

const abortableDelay = (milliseconds: number, signal?: AbortSignal) =>
  new Promise<void>((resolve, reject) => {
    const timer = window.setTimeout(resolve, milliseconds)
    signal?.addEventListener(
      "abort",
      () => {
        window.clearTimeout(timer)
        reject(new DOMException("Operation aborted", "AbortError"))
      },
      { once: true }
    )
  })

export const listDirectory = (
  agentID: string,
  rootID: string,
  relativePath: string,
  cursor: string,
  signal?: AbortSignal
) =>
  createOperation<DirectoryPage>(
    agentID,
    "directory",
    { root_id: rootID, relative_path: relativePath, cursor, page_size: 100 },
    signal
  )

export const getFileMetadata = (
  agentID: string,
  rootID: string,
  relativePath: string,
  signal?: AbortSignal
) =>
  createOperation<MetadataResult>(
    agentID,
    "metadata",
    { root_id: rootID, relative_path: relativePath },
    signal
  )

export const readFilePreview = (
  agentID: string,
  rootID: string,
  relativePath: string,
  signal?: AbortSignal
) =>
  createOperation<PreviewResult>(
    agentID,
    "preview",
    { root_id: rootID, relative_path: relativePath, offset: 0, length: 65536 },
    signal
  )

export const decodePreviewText = (preview: PreviewResult) => {
  const bytes = Uint8Array.from(atob(preview.data), (value) =>
    value.charCodeAt(0)
  )
  return new TextDecoder("utf-8", { fatal: false }).decode(bytes)
}
