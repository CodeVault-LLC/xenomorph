import { type RefObject, useEffect, useRef, useState } from "react"

import { DetailsInspector } from "@/components/dashboard/file-explorer/details-inspector"
import { FileBrowserCard } from "@/components/dashboard/file-explorer/file-browser-card"
import { FileViewer } from "@/components/dashboard/file-explorer/file-viewer"
import {
  MutationDialog,
  type MutationIntent,
} from "@/components/dashboard/file-explorer/mutation-dialog"
import {
  errorMessage,
  isAbortError,
  isTerminalTransfer,
  joinPath,
} from "@/components/dashboard/file-explorer/shared"
import { TransferDrawer } from "@/components/dashboard/file-explorer/transfer-drawer"
import { UploadDialog } from "@/components/dashboard/file-explorer/upload-dialog"
import {
  createDownloadTransfer,
  getFileMetadata,
  listDirectory,
  listTransfers,
  probeFileRoots,
  readFilePreview,
  type DirectoryPage,
  type FileEntry,
  type FileRoot,
  type FileTransfer,
  type MetadataResult,
  type MutationResult,
  type MutationVerb,
  type PreviewResult,
} from "@/lib/files"

const activeTransferPollMilliseconds = 1_000
const idleTransferPollMilliseconds = 5_000

type RootSelection = {
  agentID: string
  root: FileRoot
}

export function FileExplorer({ agentID }: { agentID: string }) {
  const [roots, setRoots] = useState<FileRoot[]>([])
  const [rootSelection, setRootSelection] = useState<RootSelection>()
  const [relativePath, setRelativePath] = useState("")
  const [page, setPage] = useState<DirectoryPage>()
  const [cursorHistory, setCursorHistory] = useState<string[]>([""])
  const [cursorIndex, setCursorIndex] = useState(0)
  const [metadata, setMetadata] = useState<MetadataResult>()
  const [preview, setPreview] = useState<PreviewResult>()
  const [selectedPath, setSelectedPath] = useState("")
  const [probeLoading, setProbeLoading] = useState(true)
  const [directoryLoading, setDirectoryLoading] = useState(false)
  const [detailsLoading, setDetailsLoading] = useState(false)
  const [downloadPending, setDownloadPending] = useState(false)
  const [uploadOpen, setUploadOpen] = useState(false)
  const [mutationIntent, setMutationIntent] = useState<MutationIntent>()
  const [selectedEntryIDs, setSelectedEntryIDs] = useState<Set<string>>(
    new Set()
  )
  const [directoryRevision, setDirectoryRevision] = useState(0)
  const [workspaceError, setWorkspaceError] = useState("")
  const [detailsError, setDetailsError] = useState("")
  const [transfers, setTransfers] = useState<FileTransfer[]>([])
  const [transferError, setTransferError] = useState("")
  const probeRequestRef = useRef<AbortController | undefined>(undefined)
  const directoryRequestRef = useRef<AbortController | undefined>(undefined)
  const detailsRequestRef = useRef<AbortController | undefined>(undefined)
  const activeAgentRef = useRef(agentID)

  const root =
    rootSelection?.agentID === agentID ? rootSelection.root : undefined

  useEffect(() => {
    activeAgentRef.current = agentID
    const controller = replaceRequest(probeRequestRef)
    directoryRequestRef.current?.abort()
    detailsRequestRef.current?.abort()
    queueMicrotask(() => {
      if (controller.signal.aborted) return
      setProbeLoading(true)
      setDirectoryLoading(false)
      setDetailsLoading(false)
      setDownloadPending(false)
      setUploadOpen(false)
      setMutationIntent(undefined)
      setSelectedEntryIDs(new Set())
      setWorkspaceError("")
      setDetailsError("")
      setRoots([])
      setRootSelection(undefined)
      setRelativePath("")
      setPage(undefined)
      setCursorHistory([""])
      setCursorIndex(0)
      setMetadata(undefined)
      setPreview(undefined)
      setSelectedPath("")
      setTransfers([])
      setTransferError("")
    })

    probeFileRoots(agentID, controller.signal)
      .then((probe) => {
        if (controller.signal.aborted) return
        const nextRoots = probe.roots
          .filter((item) => item.available)
          .map(({ root_id, display_label, allowed_verbs, capabilities }) => ({
            root_id,
            display_label,
            allowed_verbs,
            capabilities,
          }))
        setRoots(nextRoots)
        setRootSelection((current) => {
          const currentRootID =
            current?.agentID === agentID ? current.root.root_id : ""
          const nextRoot =
            nextRoots.find(
              (candidate) => candidate.root_id === currentRootID
            ) ??
            nextRoots.find((candidate) => candidate.root_id === "home") ??
            nextRoots[0]
          return nextRoot ? { agentID, root: nextRoot } : undefined
        })
      })
      .catch((cause: unknown) => {
        if (!isAbortError(cause)) setWorkspaceError(errorMessage(cause))
      })
      .finally(() => {
        if (
          !controller.signal.aborted &&
          probeRequestRef.current === controller
        ) {
          setProbeLoading(false)
        }
      })

    return () => controller.abort()
  }, [agentID])

  useEffect(() => {
    if (!root) return
    const controller = replaceRequest(directoryRequestRef)
    const cursor = cursorHistory[cursorIndex] ?? ""
    queueMicrotask(() => {
      if (controller.signal.aborted) return
      setDirectoryLoading(true)
      setWorkspaceError("")
      setPage(undefined)
    })

    listDirectory(
      agentID,
      root.root_id,
      relativePath,
      cursor,
      controller.signal
    )
      .then((nextPage) => {
        if (directoryRequestRef.current === controller) setPage(nextPage)
      })
      .catch((cause: unknown) => {
        if (
          !isAbortError(cause) &&
          directoryRequestRef.current === controller
        ) {
          setWorkspaceError(errorMessage(cause))
        }
      })
      .finally(() => {
        if (
          !controller.signal.aborted &&
          directoryRequestRef.current === controller
        ) {
          setDirectoryLoading(false)
        }
      })

    return () => controller.abort()
  }, [
    agentID,
    cursorHistory,
    cursorIndex,
    directoryRevision,
    relativePath,
    root,
  ])

  useEffect(() => {
    let active = true
    let timer = 0

    const refresh = async () => {
      let delay = idleTransferPollMilliseconds
      try {
        const items = await listTransfers(agentID)
        if (!active) return
        setTransfers(items)
        setTransferError("")
        if (items.some((item) => !isTerminalTransfer(item))) {
          delay = activeTransferPollMilliseconds
        }
      } catch (cause) {
        if (active) setTransferError(errorMessage(cause))
      } finally {
        if (active) {
          timer = window.setTimeout(() => void refresh(), delay)
        }
      }
    }

    void refresh()
    return () => {
      active = false
      window.clearTimeout(timer)
    }
  }, [agentID])

  useEffect(
    () => () => {
      probeRequestRef.current?.abort()
      directoryRequestRef.current?.abort()
      detailsRequestRef.current?.abort()
    },
    []
  )

  const clearSelectedFile = () => {
    detailsRequestRef.current?.abort()
    setMetadata(undefined)
    setPreview(undefined)
    setSelectedPath("")
    setDetailsError("")
    setDetailsLoading(false)
  }

  const resetPagination = () => {
    setCursorHistory([""])
    setCursorIndex(0)
  }

  const selectRoot = (nextRoot: FileRoot) => {
    setRootSelection({ agentID, root: nextRoot })
    setRelativePath("")
    setPage(undefined)
    resetPagination()
    clearSelectedFile()
    setSelectedEntryIDs(new Set())
  }

  const navigate = (path: string) => {
    setRelativePath(path)
    setPage(undefined)
    resetPagination()
    clearSelectedFile()
    setSelectedEntryIDs(new Set())
  }

  const openEntry = (entry: FileEntry) => {
    if (!root || !entry.operation_name) return
    const path = relativePath
      ? `${relativePath}/${entry.operation_name}`
      : entry.operation_name
    if (entry.kind === "directory") {
      navigate(path)
      return
    }

    const controller = replaceRequest(detailsRequestRef)
    setSelectedPath(path)
    setMetadata(undefined)
    setPreview(undefined)
    setDetailsLoading(true)
    setDetailsError("")
    Promise.all([
      getFileMetadata(agentID, root.root_id, path, controller.signal),
      entry.kind === "file" && root.allowed_verbs.includes("preview")
        ? readFilePreview(agentID, root.root_id, path, controller.signal)
        : Promise.resolve(undefined),
    ])
      .then(([nextMetadata, nextPreview]) => {
        if (detailsRequestRef.current !== controller) return
        setMetadata(nextMetadata)
        setPreview(nextPreview)
      })
      .catch((cause: unknown) => {
        if (!isAbortError(cause) && detailsRequestRef.current === controller) {
          setDetailsError(errorMessage(cause))
        }
      })
      .finally(() => {
        if (
          !controller.signal.aborted &&
          detailsRequestRef.current === controller
        ) {
          setDetailsLoading(false)
        }
      })
  }

  const downloadSelectedFile = async () => {
    if (
      !root ||
      metadata?.kind !== "file" ||
      !selectedPath ||
      downloadPending
    ) {
      return
    }
    const requestAgentID = agentID
    setDownloadPending(true)
    setTransferError("")
    try {
      const transfer = await createDownloadTransfer(
        requestAgentID,
        root.root_id,
        selectedPath
      )
      if (activeAgentRef.current === requestAgentID) {
        setTransfers((current) => [transfer, ...current])
      }
    } catch (cause) {
      if (activeAgentRef.current === requestAgentID) {
        setTransferError(errorMessage(cause))
      }
    } finally {
      if (activeAgentRef.current === requestAgentID) setDownloadPending(false)
    }
  }

  const nextPage = () => {
    const nextCursor = page?.next_cursor
    if (!nextCursor) return
    setPage(undefined)
    setCursorHistory((current) => [
      ...current.slice(0, cursorIndex + 1),
      nextCursor,
    ])
    setCursorIndex((current) => current + 1)
    setSelectedEntryIDs(new Set())
  }

  const previousPage = () => {
    setPage(undefined)
    setCursorIndex((current) => Math.max(0, current - 1))
    setSelectedEntryIDs(new Set())
  }

  const refreshDirectory = () => {
    setPage(undefined)
    setDirectoryRevision((current) => current + 1)
  }

  const selectEntry = (entry: FileEntry, selected: boolean) => {
    setSelectedEntryIDs((current) => {
      const next = new Set(current)
      if (selected) next.add(entry.entry_id)
      else next.delete(entry.entry_id)
      return next
    })
  }

  const locatedEntry = (entry: FileEntry) => ({
    entry,
    path: joinPath(relativePath, entry.operation_name ?? ""),
  })

  const requestMutation = (verb: MutationVerb, entry: FileEntry) => {
    if (!entry.operation_name) return
    setMutationIntent({
      verb,
      entries: [locatedEntry(entry)],
      directory: relativePath,
    })
  }

  const requestCreate = (verb: "create_file" | "create_directory") => {
    setMutationIntent({ verb, entries: [], directory: relativePath })
  }

  const requestBulkDelete = () => {
    const entries = (page?.entries ?? [])
      .filter(
        (entry) => selectedEntryIDs.has(entry.entry_id) && entry.operation_name
      )
      .map(locatedEntry)
    if (entries.length > 0) {
      setMutationIntent({ verb: "delete", entries, directory: relativePath })
    }
  }

  const mutationCompleted = (result: MutationResult) => {
    if (result.items.some((item) => item.state === "completed")) {
      setSelectedEntryIDs(new Set())
      clearSelectedFile()
      refreshDirectory()
    }
  }

  const upsertTransfer = (transfer: FileTransfer) => {
    setTransfers((current) => {
      const exists = current.some(
        (item) => item.transfer_id === transfer.transfer_id
      )
      return exists
        ? current.map((item) =>
            item.transfer_id === transfer.transfer_id ? transfer : item
          )
        : [transfer, ...current]
    })
  }

  return (
    <>
      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
        {selectedPath ? (
          <FileViewer
            preview={preview}
            selectedPath={selectedPath}
            loading={detailsLoading}
            error={detailsError}
            canDownload={
              root?.allowed_verbs.includes("transfer") === true &&
              metadata?.kind === "file"
            }
            downloadPending={downloadPending}
            onBack={clearSelectedFile}
            onDownload={() => void downloadSelectedFile()}
          />
        ) : (
          <FileBrowserCard
            roots={roots}
            root={root}
            relativePath={relativePath}
            page={page}
            loading={
              probeLoading ||
              directoryLoading ||
              (root !== undefined &&
                page === undefined &&
                workspaceError === "")
            }
            error={workspaceError}
            canTransfer={root?.allowed_verbs.includes("transfer") === true}
            canMutate={root?.allowed_verbs.includes("mutate") === true}
            canDelete={
              root?.allowed_verbs.includes("mutate") === true &&
              root.capabilities.permanent_delete === "available"
            }
            selectedEntryIDs={selectedEntryIDs}
            cursorIndex={cursorIndex}
            onUpload={() => setUploadOpen(true)}
            onCreate={requestCreate}
            onBulkDelete={requestBulkDelete}
            onRootChange={selectRoot}
            onNavigate={navigate}
            onOpen={openEntry}
            onSelectionChange={selectEntry}
            onAction={requestMutation}
            onPreviousPage={previousPage}
            onNextPage={nextPage}
          />
        )}
        <div className="flex flex-col gap-4">
          <DetailsInspector
            metadata={metadata}
            selectedPath={selectedPath}
            loading={detailsLoading}
            error={detailsError}
          />
          <TransferDrawer
            key={agentID}
            agentID={agentID}
            transfers={transfers}
            error={transferError}
            onChange={(transfer) => {
              if (activeAgentRef.current === agentID) {
                setTransfers((current) =>
                  current.map((item) =>
                    item.transfer_id === transfer.transfer_id ? transfer : item
                  )
                )
              }
            }}
            onRemove={(transferID) => {
              if (activeAgentRef.current === agentID) {
                setTransfers((current) =>
                  current.filter((item) => item.transfer_id !== transferID)
                )
              }
            }}
            onRemoveFinished={() => {
              if (activeAgentRef.current === agentID) {
                setTransfers((current) =>
                  current.filter((item) => !isTerminalTransfer(item))
                )
              }
            }}
          />
        </div>
      </div>
      {root ? (
        <>
          <UploadDialog
            open={uploadOpen}
            agentID={agentID}
            rootID={root.root_id}
            directory={relativePath}
            allowReplace={root.capabilities.atomic_rename === "available"}
            onOpenChange={setUploadOpen}
            onTransfer={upsertTransfer}
            onComplete={refreshDirectory}
          />
          <MutationDialog
            intent={mutationIntent}
            agentID={agentID}
            rootID={root.root_id}
            allowReplace={root.capabilities.atomic_rename === "available"}
            onOpenChange={(open) => {
              if (!open) setMutationIntent(undefined)
            }}
            onComplete={mutationCompleted}
          />
        </>
      ) : null}
    </>
  )
}

function replaceRequest(ref: RefObject<AbortController | undefined>) {
  ref.current?.abort()
  const controller = new AbortController()
  ref.current = controller
  return controller
}
