import { Archive, CheckCircle2, FolderInput, ListTree } from "lucide-react"
import { useEffect, useRef, useState } from "react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { executeArchive, type ArchiveResult } from "@/lib/files"

import {
  errorMessage,
  isAbortError,
  isValidRelativePath,
  joinPath,
} from "./shared"
import { formatBytes } from "./shared"

export type ArchiveIntent =
  | { action: "create"; directory: string; sources: string[] }
  | { action: "list" | "extract"; directory: string; archivePath: string }

const conflictOptions = [
  { value: "fail", label: "Stop if an output exists" },
  { value: "skip", label: "Skip existing outputs" },
  { value: "rename_new", label: "Keep both with a new name" },
] as const

export function ArchiveDialog({
  intent,
  agentID,
  rootID,
  onOpenChange,
  onComplete,
}: {
  intent?: ArchiveIntent
  agentID: string
  rootID: string
  onOpenChange: (open: boolean) => void
  onComplete: () => void
}) {
  const [archivePath, setArchivePath] = useState(() =>
    intent?.action === "create"
      ? joinPath(intent.directory, "archive.zip")
      : (intent?.archivePath ?? "")
  )
  const [destination, setDestination] = useState(() =>
    intent?.action === "extract" ? intent.directory || "extracted" : ""
  )
  const [conflict, setConflict] = useState<"fail" | "skip" | "rename_new">(
    "fail"
  )
  const [pending, setPending] = useState(false)
  const [error, setError] = useState("")
  const [result, setResult] = useState<ArchiveResult>()
  const controllerRef = useRef<AbortController | undefined>(undefined)

  useEffect(() => () => controllerRef.current?.abort(), [])

  if (!intent) return null

  const archivePathError = !isValidRelativePath(archivePath)
    ? "Use a normalized root-relative .zip path."
    : !archivePath.toLocaleLowerCase().endsWith(".zip")
      ? "The archive filename must end in .zip."
      : ""
  const destinationError =
    intent.action === "extract" && !isValidRelativePath(destination)
      ? "Choose an existing normalized root-relative directory."
      : ""

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (archivePathError || destinationError || pending) return
    const controller = new AbortController()
    controllerRef.current = controller
    setPending(true)
    setError("")
    setResult(undefined)
    try {
      const next = await executeArchive(
        agentID,
        rootID,
        {
          action: intent.action,
          archive_path: archivePath,
          destination_path:
            intent.action === "extract" ? destination : undefined,
          source_paths: intent.action === "create" ? intent.sources : undefined,
          conflict_strategy: intent.action === "list" ? "fail" : conflict,
        },
        controller.signal
      )
      setResult(next)
      if (intent.action !== "list" && next.state === "completed") onComplete()
    } catch (cause) {
      if (!isAbortError(cause)) setError(errorMessage(cause))
    } finally {
      controllerRef.current = undefined
      setPending(false)
    }
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!pending) onOpenChange(open)
      }}
    >
      <DialogContent className="sm:max-w-xl" showCloseButton={!pending}>
        <DialogHeader>
          <DialogTitle>{archiveTitle(intent.action)}</DialogTitle>
          <DialogDescription>{archiveDescription(intent)}</DialogDescription>
        </DialogHeader>
        <form className="flex flex-col gap-5" onSubmit={submit}>
          <FieldGroup>
            <Field data-invalid={archivePathError !== ""}>
              <FieldLabel htmlFor="archive-path">Archive path</FieldLabel>
              <Input
                id="archive-path"
                value={archivePath}
                readOnly={intent.action !== "create"}
                disabled={pending}
                aria-invalid={archivePathError !== ""}
                onChange={(event) => setArchivePath(event.target.value)}
              />
              <FieldDescription>
                ZIP is the only enabled format. Links and special entries are
                rejected rather than preserved.
              </FieldDescription>
              <FieldError>{archivePathError}</FieldError>
            </Field>
            {intent.action === "extract" ? (
              <Field data-invalid={destinationError !== ""}>
                <FieldLabel htmlFor="archive-destination">
                  Destination directory
                </FieldLabel>
                <Input
                  id="archive-destination"
                  value={destination}
                  disabled={pending}
                  aria-invalid={destinationError !== ""}
                  onChange={(event) => setDestination(event.target.value)}
                />
                <FieldDescription>
                  Extraction stays below this existing directory and validates
                  the complete archive before writing.
                </FieldDescription>
                <FieldError>{destinationError}</FieldError>
              </Field>
            ) : null}
            {intent.action !== "list" ? (
              <Field>
                <FieldLabel>When an output exists</FieldLabel>
                <Select
                  items={conflictOptions}
                  value={conflict}
                  disabled={pending}
                  onValueChange={(value) => {
                    if (value) setConflict(value)
                  }}
                >
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {conflictOptions.map((option) => (
                        <SelectItem key={option.value} value={option.value}>
                          {option.label}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>
            ) : null}
          </FieldGroup>
          {error ? (
            <Alert variant="destructive">
              <AlertTitle>Archive operation failed</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          {result ? (
            <ArchiveOutcome action={intent.action} result={result} />
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={pending}
              onClick={() => onOpenChange(false)}
            >
              {result ? "Close" : "Cancel"}
            </Button>
            <Button
              type="submit"
              disabled={
                pending ||
                archivePathError !== "" ||
                destinationError !== "" ||
                !!result
              }
            >
              {pending ? (
                <Spinner data-icon="inline-start" />
              ) : intent.action === "list" ? (
                <ListTree data-icon="inline-start" />
              ) : intent.action === "extract" ? (
                <FolderInput data-icon="inline-start" />
              ) : (
                <Archive data-icon="inline-start" />
              )}
              {pending ? "Working…" : archiveSubmitLabel(intent.action)}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function ArchiveOutcome({
  action,
  result,
}: {
  action: ArchiveIntent["action"]
  result: ArchiveResult
}) {
  if (action === "list") {
    return (
      <div className="flex flex-col gap-2">
        <p className="text-sm text-muted-foreground" role="status">
          {result.entries_processed.toLocaleString()} safe entries ·{" "}
          {formatBytes(result.bytes_processed)} expanded
          {result.truncated ? " · list display truncated" : ""}
        </p>
        <ScrollArea className="h-64 rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Entry</TableHead>
                <TableHead>Kind</TableHead>
                <TableHead>Expanded</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(result.entries ?? []).map((entry, index) => (
                <TableRow key={`${index}:${entry.kind}:${entry.path}`}>
                  <TableCell className="max-w-72 truncate font-mono text-xs">
                    {entry.path}
                  </TableCell>
                  <TableCell>{entry.kind}</TableCell>
                  <TableCell>{formatBytes(entry.uncompressed_size)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </ScrollArea>
      </div>
    )
  }
  return (
    <Alert>
      <CheckCircle2 />
      <AlertTitle>
        {result.state === "skipped"
          ? "Archive output skipped"
          : "Archive operation completed"}
      </AlertTitle>
      <AlertDescription>
        {result.entries_processed.toLocaleString()} entries ·{" "}
        {formatBytes(result.bytes_processed)} processed
      </AlertDescription>
    </Alert>
  )
}

function archiveTitle(action: ArchiveIntent["action"]) {
  if (action === "create") return "Create ZIP archive"
  if (action === "extract") return "Extract ZIP archive"
  return "Inspect ZIP archive"
}

function archiveSubmitLabel(action: ArchiveIntent["action"]) {
  if (action === "create") return "Create archive"
  if (action === "extract") return "Extract safely"
  return "Inspect archive"
}

function archiveDescription(intent: ArchiveIntent) {
  if (intent.action === "create") {
    return `Create a bounded archive from ${intent.sources.length.toLocaleString()} selected item${intent.sources.length === 1 ? "" : "s"}.`
  }
  if (intent.action === "extract") {
    return "Review and extract regular files and directories under fixed traversal, expansion, and runtime limits."
  }
  return "Inspect bounded, normalized entry metadata without extracting or previewing archive content."
}
