import { type MutableRefObject, useEffect, useRef, useState } from "react"
import {
  ChevronLeft,
  ChevronRight,
  File,
  FileQuestion,
  Folder,
  FolderOpen,
  HardDrive,
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
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  decodePreviewText,
  getFileMetadata,
  listDirectory,
  probeFileRoots,
  readFilePreview,
  type DirectoryPage,
  type FileEntry,
  type FileRoot,
  type MetadataResult,
  type PreviewResult,
  type RootObservation,
} from "@/lib/files"

export function FileExplorer({ agentID }: { agentID: string }) {
  const [roots, setRoots] = useState<FileRoot[]>([])
  const [rootObservations, setRootObservations] = useState<RootObservation[]>(
    []
  )
  const [root, setRoot] = useState<FileRoot>()
  const [rootAgentID, setRootAgentID] = useState("")
  const [relativePath, setRelativePath] = useState("")
  const [page, setPage] = useState<DirectoryPage>()
  const [cursorHistory, setCursorHistory] = useState<string[]>([""])
  const [cursorIndex, setCursorIndex] = useState(0)
  const [metadata, setMetadata] = useState<MetadataResult>()
  const [preview, setPreview] = useState<PreviewResult>()
  const [selectedPath, setSelectedPath] = useState("")
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState("")
  const requestRef = useRef<AbortController | undefined>(undefined)

  useEffect(() => () => requestRef.current?.abort(), [])

  useEffect(() => {
    const controller = beginRequest(requestRef)
    queueMicrotask(() => {
      if (!controller.signal.aborted) {
        setLoading(true)
        setError("")
        setRoots([])
        setRoot(undefined)
        setRootAgentID("")
        setPage(undefined)
      }
    })
    probeFileRoots(agentID, controller.signal)
      .then((probe) => {
        const nextRoots = probe.roots
          .filter((item) => item.available)
          .map((item) => ({
            root_id: item.root_id,
            display_label: item.display_label,
            allowed_verbs: item.allowed_verbs,
          }))
        setRoots(nextRoots)
        setRootObservations(probe.roots)
        setRootAgentID(agentID)
        setRoot(
          (current) =>
            nextRoots.find(
              (candidate) =>
                candidate.root_id === current?.root_id &&
                probe.roots.find((item) => item.root_id === candidate.root_id)
                  ?.available !== false
            ) ??
            nextRoots.find(
              (candidate) =>
                probe.roots.find((item) => item.root_id === candidate.root_id)
                  ?.available !== false
            )
        )
      })
      .catch((cause: unknown) => handleRequestError(cause, setError))
      .finally(() => setLoading(false))
    return () => controller.abort()
  }, [agentID])

  useEffect(() => {
    if (!root || rootAgentID !== agentID) return
    const controller = beginRequest(requestRef)
    queueMicrotask(() => {
      if (!controller.signal.aborted) {
        setLoading(true)
        setError("")
        setMetadata(undefined)
        setPreview(undefined)
      }
    })
    const cursor = cursorHistory[cursorIndex] ?? ""
    listDirectory(
      agentID,
      root.root_id,
      relativePath,
      cursor,
      controller.signal
    )
      .then(setPage)
      .catch((cause: unknown) => handleRequestError(cause, setError))
      .finally(() => setLoading(false))
    return () => controller.abort()
  }, [agentID, cursorHistory, cursorIndex, relativePath, root, rootAgentID])

  return (
    <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
      <Card className="min-w-0">
        <CardHeader>
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="flex flex-col gap-1">
              <CardTitle>Files</CardTitle>
              <CardDescription>
                Read-only filesystem observations from this agent.
              </CardDescription>
            </div>
            <Badge variant="outline">No-follow browsing</Badge>
          </div>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex flex-wrap gap-2" aria-label="Filesystem roots">
            {roots.map((candidate) => {
              const observation = rootObservations.find(
                (item) => item.root_id === candidate.root_id
              )
              return (
                <Button
                  key={candidate.root_id}
                  variant={
                    candidate.root_id === root?.root_id
                      ? "secondary"
                      : "outline"
                  }
                  size="sm"
                  disabled={observation?.available === false}
                  onClick={() => {
                    setRoot(candidate)
                    setRelativePath("")
                    resetPagination(setCursorHistory, setCursorIndex)
                  }}
                >
                  <HardDrive data-icon="inline-start" />
                  {candidate.display_label}
                </Button>
              )
            })}
          </div>

          <PathNavigation
            relativePath={relativePath}
            onNavigate={(path) => {
              setRelativePath(path)
              resetPagination(setCursorHistory, setCursorIndex)
            }}
          />

          {error ? (
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <FileQuestion />
                </EmptyMedia>
                <EmptyTitle>Workspace request failed</EmptyTitle>
                <EmptyDescription>{error}</EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : loading && !page ? (
            <div className="flex flex-col gap-2" aria-label="Loading directory">
              {Array.from({ length: 6 }, (_, index) => (
                <Skeleton key={index} className="h-10 w-full" />
              ))}
            </div>
          ) : !root ? (
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <HardDrive />
                </EmptyMedia>
                <EmptyTitle>No filesystem roots available</EmptyTitle>
                <EmptyDescription>
                  The agent did not report a readable filesystem root.
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : page?.entries.length === 0 ? (
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <FolderOpen />
                </EmptyMedia>
                <EmptyTitle>Directory is empty</EmptyTitle>
                <EmptyDescription>
                  No entries were observed in this snapshot.
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <DirectoryTable
              entries={page?.entries ?? []}
              onOpen={(entry) =>
                openEntry(entry, {
                  agentID,
                  root,
                  relativePath,
                  setRelativePath,
                  setMetadata,
                  setPreview,
                  setSelectedPath,
                  setError,
                  requestRef,
                  setLoading,
                  setCursorHistory,
                  setCursorIndex,
                })
              }
            />
          )}

          <div className="flex items-center justify-between gap-3">
            <span className="text-xs text-muted-foreground">
              {page
                ? `${page.entries.length} visible entries · ${page.ordering}`
                : "No snapshot"}
            </span>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={cursorIndex === 0 || loading}
                onClick={() =>
                  setCursorIndex((current) => Math.max(0, current - 1))
                }
              >
                <ChevronLeft data-icon="inline-start" /> Previous
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={!page?.has_more || !page.next_cursor || loading}
                onClick={() => {
                  if (!page?.next_cursor) return
                  setCursorHistory((current) => [
                    ...current.slice(0, cursorIndex + 1),
                    page.next_cursor!,
                  ])
                  setCursorIndex((current) => current + 1)
                }}
              >
                Next <ChevronRight data-icon="inline-end" />
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      <DetailsInspector
        metadata={metadata}
        preview={preview}
        selectedPath={selectedPath}
        loading={loading}
      />
    </div>
  )
}

function PathNavigation({
  relativePath,
  onNavigate,
}: {
  relativePath: string
  onNavigate: (path: string) => void
}) {
  const parts = relativePath ? relativePath.split("/") : []
  return (
    <nav
      className="flex flex-wrap items-center gap-1"
      aria-label="Current filesystem path"
    >
      <Button variant="ghost" size="sm" onClick={() => onNavigate("")}>
        <HardDrive data-icon="inline-start" /> Root
      </Button>
      {parts.map((part, index) => (
        <div key={`${part}-${index}`} className="flex items-center gap-1">
          <ChevronRight aria-hidden="true" />
          <Button
            variant="ghost"
            size="sm"
            onClick={() => onNavigate(parts.slice(0, index + 1).join("/"))}
          >
            {part}
          </Button>
        </div>
      ))}
    </nav>
  )
}

function DirectoryTable({
  entries,
  onOpen,
}: {
  entries: FileEntry[]
  onOpen: (entry: FileEntry) => void
}) {
  return (
    <div className="max-h-[520px] overflow-auto rounded-lg border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Kind</TableHead>
            <TableHead>Size</TableHead>
            <TableHead>Modified</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {entries.map((entry) => {
            const EntryIcon = entry.kind === "directory" ? Folder : File
            return (
              <TableRow key={entry.entry_id}>
                <TableCell>
                  <Button
                    variant="link"
                    className="max-w-[420px] justify-start px-0"
                    disabled={!entry.operation_name}
                    title={
                      entry.operation_name
                        ? undefined
                        : "This native name cannot be addressed safely through the normalized path protocol"
                    }
                    onClick={() => onOpen(entry)}
                  >
                    <EntryIcon data-icon="inline-start" />
                    <span className="truncate">{entry.display_name}</span>
                  </Button>
                </TableCell>
                <TableCell>
                  <Badge variant="outline">{entry.kind}</Badge>
                </TableCell>
                <TableCell>
                  {entry.kind === "file" ? formatBytes(entry.size) : "—"}
                </TableCell>
                <TableCell>
                  {new Intl.DateTimeFormat(undefined, {
                    dateStyle: "medium",
                    timeStyle: "short",
                  }).format(new Date(entry.modified_at))}
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}

function DetailsInspector({
  metadata,
  preview,
  selectedPath,
  loading,
}: {
  metadata?: MetadataResult
  preview?: PreviewResult
  selectedPath: string
  loading: boolean
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Details</CardTitle>
        <CardDescription>
          {selectedPath ||
            "Select an entry to inspect client-authored metadata."}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {loading && selectedPath ? (
          <Skeleton className="h-28 w-full" />
        ) : metadata ? (
          <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-2 text-sm">
            <dt className="text-muted-foreground">Kind</dt>
            <dd>{metadata.kind}</dd>
            <dt className="text-muted-foreground">Size</dt>
            <dd>{formatBytes(metadata.size)}</dd>
            <dt className="text-muted-foreground">Modified</dt>
            <dd>{new Date(metadata.modified_at).toLocaleString()}</dd>
            <dt className="text-muted-foreground">Mode</dt>
            <dd className="font-mono">{metadata.mode.toString(8)}</dd>
          </dl>
        ) : (
          <p className="text-sm text-muted-foreground">No entry selected.</p>
        )}
        {preview && (
          <div className="flex flex-col gap-2">
            <div className="flex items-center justify-between gap-2">
              <span className="text-sm font-medium">Bounded preview</span>
              <Badge variant="outline">{preview.classification}</Badge>
            </div>
            {preview.classification === "text" ? (
              <pre className="max-h-72 overflow-auto rounded-lg bg-muted p-3 text-xs whitespace-pre-wrap">
                {decodePreviewText(preview)}
              </pre>
            ) : (
              <p className="text-sm text-muted-foreground">
                Binary content is not rendered.
              </p>
            )}
            {preview.truncated && (
              <span className="text-xs text-muted-foreground">
                Preview truncated by policy.
              </span>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

type OpenEntryContext = {
  agentID: string
  root: FileRoot
  relativePath: string
  setRelativePath: (value: string) => void
  setMetadata: (value?: MetadataResult) => void
  setPreview: (value?: PreviewResult) => void
  setSelectedPath: (value: string) => void
  setError: (value: string) => void
  requestRef: MutableRefObject<AbortController | undefined>
  setLoading: (value: boolean) => void
  setCursorHistory: (value: string[]) => void
  setCursorIndex: (value: number) => void
}

function openEntry(entry: FileEntry, context: OpenEntryContext) {
  if (!entry.operation_name) return
  const path = context.relativePath
    ? `${context.relativePath}/${entry.operation_name}`
    : entry.operation_name
  if (entry.kind === "directory") {
    context.setRelativePath(path)
    resetPagination(context.setCursorHistory, context.setCursorIndex)
    return
  }
  const controller = beginRequest(context.requestRef)
  context.setSelectedPath(path)
  context.setMetadata(undefined)
  context.setPreview(undefined)
  context.setLoading(true)
  context.setError("")
  Promise.all([
    getFileMetadata(
      context.agentID,
      context.root.root_id,
      path,
      controller.signal
    ),
    entry.kind === "file" && context.root.allowed_verbs.includes("preview")
      ? readFilePreview(
          context.agentID,
          context.root.root_id,
          path,
          controller.signal
        )
      : Promise.resolve(undefined),
  ])
    .then(([nextMetadata, nextPreview]) => {
      context.setMetadata(nextMetadata)
      context.setPreview(nextPreview)
    })
    .catch((cause: unknown) => handleRequestError(cause, context.setError))
    .finally(() => context.setLoading(false))
}

function beginRequest(ref: MutableRefObject<AbortController | undefined>) {
  ref.current?.abort()
  const controller = new AbortController()
  ref.current = controller
  return controller
}

function handleRequestError(cause: unknown, setError: (value: string) => void) {
  if (cause instanceof DOMException && cause.name === "AbortError") return
  setError(
    cause instanceof Error ? cause.message : "File workspace request failed"
  )
}

function resetPagination(
  setHistory: (value: string[]) => void,
  setIndex: (value: number) => void
) {
  setHistory([""])
  setIndex(0)
}

function formatBytes(value: number) {
  return new Intl.NumberFormat(undefined, {
    style: "unit",
    unit: "byte",
    notation: "compact",
    unitDisplay: "narrow",
  }).format(value)
}
