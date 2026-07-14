import { sha256 } from "@noble/hashes/sha2.js"
import { bytesToHex } from "@noble/hashes/utils.js"

export type FileVerb = "list" | "metadata" | "preview" | "transfer" | "mutate"

export interface FileRoot {
  root_id: string
  display_label: string
  allowed_verbs: FileVerb[]
  capabilities: RootCapabilities
}

export interface RootCapabilities {
  operating_system: string
  no_follow_resolution: "available" | "unavailable" | "denied"
  safe_handle_relative_io: "available" | "unavailable" | "denied"
  atomic_rename: "available" | "unavailable" | "denied"
  permanent_delete: "available" | "unavailable" | "denied"
}

export interface RootObservation {
  root_id: string
  display_label: string
  allowed_verbs: FileVerb[]
  available: boolean
  read_only: boolean
  error_class?: string
  capabilities: RootCapabilities
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

export interface DirectorySearchResult {
  protocol_version: number
  root_id: string
  relative_path: string
  query: string
  entries: Array<{ relative_path: string; entry: FileEntry }>
  scanned_entries: number
  truncated: boolean
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
    if (signal?.aborted) {
      reject(new DOMException("Operation aborted", "AbortError"))
      return
    }
    const onAbort = () => {
      window.clearTimeout(timer)
      reject(new DOMException("Operation aborted", "AbortError"))
    }
    const timer = window.setTimeout(() => {
      signal?.removeEventListener("abort", onAbort)
      resolve()
    }, milliseconds)
    signal?.addEventListener("abort", onAbort, { once: true })
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

export const searchDirectory = (
  agentID: string,
  rootID: string,
  relativePath: string,
  query: string,
  signal?: AbortSignal
) =>
  createOperation<DirectorySearchResult>(
    agentID,
    "search",
    { root_id: rootID, relative_path: relativePath, query },
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

export type TransferState =
  | "staging"
  | "queued"
  | "running"
  | "paused"
  | "completed"
  | "failed"
  | "cancelled"

export interface FileTransfer {
  transfer_id: string
  state: TransferState
  bytes_verified: number
  acknowledged_chunks: number[]
  error_class?: string
  manifest: {
    direction: "upload" | "download"
    relative_path: string
    size: number
    chunks: TransferChunk[]
  }
}

interface TransferChunk {
  index: number
  offset: number
  size: number
  sha256: string
}

export type ConflictStrategy = "fail" | "skip" | "rename_new" | "replace"

export type MutationVerb =
  | "create_file"
  | "create_directory"
  | "rename"
  | "move"
  | "copy"
  | "duplicate"
  | "touch"
  | "truncate"
  | "append"
  | "delete"

export interface FilePreconditions {
  must_exist?: boolean
  expected_kind?: FileEntryKind
  expected_size?: number
  expected_modified_at?: string
}

export interface MutationItem {
  item_id: string
  source_path?: string
  destination_path?: string
  append_data?: string
  truncate_size?: number
  preconditions: FilePreconditions
}

export interface MutationItemResult {
  item_id: string
  state: "planned" | "completed" | "skipped" | "failed"
  error_class?: string
  result_path?: string
  bytes_applied?: number
}

export interface MutationResult {
  protocol_version: number
  operation_id: string
  dry_run: boolean
  items: MutationItemResult[]
}

export interface UploadProgress {
  phase: "hashing" | "staging" | "committing"
  bytesComplete: number
  bytesTotal: number
  transfer?: FileTransfer
}

const transferChunkSize = 4 * 1024 * 1024
const maxUploadBytes = 1024 * 1024 * 1024

export const createDownloadTransfer = async (
  agentID: string,
  rootID: string,
  relativePath: string
) => {
  const response = await fetch(`/api/clients/${agentID}/files/transfers`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({
      manifest: {
        direction: "download",
        root_id: rootID,
        relative_path: relativePath,
        chunk_size: transferChunkSize,
        conflict_strategy: "fail",
        preconditions: {},
      },
    }),
  })
  return (await readJSON<{ transfer: FileTransfer }>(response)).transfer
}

export const uploadFile = async (
  agentID: string,
  rootID: string,
  relativePath: string,
  file: File,
  conflict: ConflictStrategy,
  onProgress: (progress: UploadProgress) => void,
  signal?: AbortSignal
) => {
  if (file.size > maxUploadBytes) {
    throw new Error("Uploads are limited to 1 GiB per file")
  }

  const chunks: TransferChunk[] = []
  const objectHash = sha256.create()
  for (let offset = 0, index = 0; offset < file.size; index += 1) {
    signal?.throwIfAborted()
    const end = Math.min(offset + transferChunkSize, file.size)
    const bytes = new Uint8Array(await file.slice(offset, end).arrayBuffer())
    objectHash.update(bytes)
    chunks.push({
      index,
      offset,
      size: bytes.byteLength,
      sha256: bytesToHex(sha256(bytes)),
    })
    offset = end
    onProgress({
      phase: "hashing",
      bytesComplete: offset,
      bytesTotal: file.size,
    })
  }

  const created = await requestTransfer(
    agentID,
    {
      direction: "upload",
      root_id: rootID,
      relative_path: relativePath,
      size: file.size,
      chunk_size: transferChunkSize,
      sha256: bytesToHex(objectHash.digest()),
      chunks,
      conflict_strategy: conflict,
      preconditions: {},
    },
    signal
  )
  onProgress({
    phase: "staging",
    bytesComplete: created.bytes_verified,
    bytesTotal: file.size,
    transfer: created,
  })

  let transfer = created
  for (const chunk of chunks) {
    signal?.throwIfAborted()
    if ((transfer.acknowledged_chunks ?? []).includes(chunk.index)) continue
    const body = file.slice(chunk.offset, chunk.offset + chunk.size)
    transfer = await stageUploadChunk(
      agentID,
      transfer.transfer_id,
      chunk.index,
      body,
      signal
    )
    onProgress({
      phase: "staging",
      bytesComplete: transfer.bytes_verified,
      bytesTotal: file.size,
      transfer,
    })
  }

  onProgress({
    phase: "committing",
    bytesComplete: file.size,
    bytesTotal: file.size,
    transfer,
  })
  const committed = await commitUpload(agentID, transfer.transfer_id, signal)
  onProgress({
    phase: "committing",
    bytesComplete: file.size,
    bytesTotal: file.size,
    transfer: committed,
  })
  return committed
}

const requestTransfer = async (
  agentID: string,
  manifest: Record<string, unknown>,
  signal?: AbortSignal
) => {
  const response = await fetch(`/api/clients/${agentID}/files/transfers`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ manifest }),
    signal,
  })
  return (await readJSON<{ transfer: FileTransfer }>(response)).transfer
}

const stageUploadChunk = async (
  agentID: string,
  transferID: string,
  index: number,
  body: Blob,
  signal?: AbortSignal
) => {
  const endpoint = `/api/clients/${agentID}/files/transfers/${transferID}/chunks/${index}`
  for (let attempt = 0; attempt < 3; attempt += 1) {
    try {
      const response = await fetch(endpoint, { method: "PUT", body, signal })
      if (response.ok || response.status < 500 || attempt === 2) {
        return (await readJSON<{ transfer: FileTransfer }>(response)).transfer
      }
      await response.body?.cancel()
    } catch (cause) {
      if (signal?.aborted || attempt === 2) throw cause
    }
    await abortableDelay(250 * 2 ** attempt, signal)
  }
  throw new Error("Upload chunk retry limit was exceeded")
}

const commitUpload = async (
  agentID: string,
  transferID: string,
  signal?: AbortSignal
) => {
  const response = await fetch(
    `/api/clients/${agentID}/files/transfers/${transferID}/commit`,
    { method: "POST", signal }
  )
  return (await readJSON<{ transfer: FileTransfer }>(response)).transfer
}

export const mutateFiles = async (
  agentID: string,
  rootID: string,
  verb: MutationVerb,
  items: MutationItem[],
  conflict: ConflictStrategy,
  dryRun: boolean,
  signal?: AbortSignal
) => {
  const response = await fetch(`/api/clients/${agentID}/files/mutations`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({
      root_id: rootID,
      verb,
      dry_run: dryRun,
      conflict_strategy: conflict,
      items,
    }),
    signal,
  })
  const created = await readJSON<{ operation: FileOperation<MutationResult> }>(
    response
  )
  return pollOperation<MutationResult>(
    agentID,
    created.operation.operation_id,
    signal
  )
}

export const listTransfers = async (agentID: string) => {
  const response = await fetch(`/api/clients/${agentID}/files/transfers`, {
    cache: "no-store",
  })
  return (await readJSON<{ transfers: FileTransfer[] }>(response)).transfers
}

export const controlTransfer = async (
  agentID: string,
  transferID: string,
  action: "resume" | "abort"
) => {
  const response = await fetch(
    `/api/clients/${agentID}/files/transfers/${transferID}/${action}`,
    { method: "POST" }
  )
  return (await readJSON<{ transfer: FileTransfer }>(response)).transfer
}

export const removeTransfer = async (agentID: string, transferID: string) => {
  const response = await fetch(
    `/api/clients/${agentID}/files/transfers/${transferID}`,
    { method: "DELETE" }
  )
  return (await readJSON<{ removed: number }>(response)).removed
}

export const removeFinishedTransfers = async (agentID: string) => {
  const response = await fetch(`/api/clients/${agentID}/files/transfers`, {
    method: "DELETE",
  })
  return (await readJSON<{ removed: number }>(response)).removed
}

export const transferContentURL = (agentID: string, transferID: string) =>
  `/api/clients/${agentID}/files/transfers/${transferID}/content`
