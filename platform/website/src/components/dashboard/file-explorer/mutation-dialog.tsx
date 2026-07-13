import { useEffect, useMemo, useRef, useState } from "react"
import { CheckCircle2, FilePenLine, Trash2, TriangleAlert } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
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
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { Spinner } from "@/components/ui/spinner"
import {
  mutateFiles,
  type ConflictStrategy,
  type FileEntry,
  type MutationItem,
  type MutationResult,
  type MutationVerb,
} from "@/lib/files"

import {
  errorMessage,
  isAbortError,
  isValidRelativePath,
  joinPath,
} from "./shared"

export type LocatedEntry = { entry: FileEntry; path: string }

export type MutationIntent = {
  verb: MutationVerb
  entries: LocatedEntry[]
  directory: string
}

const conflictOptions = [
  { label: "Stop on conflict", value: "fail" },
  { label: "Skip conflicting items", value: "skip" },
  { label: "Keep both", value: "rename_new" },
  { label: "Replace destination", value: "replace" },
] as const

type MutationDialogProps = {
  intent?: MutationIntent
  agentID: string
  rootID: string
  allowReplace: boolean
  onOpenChange: (open: boolean) => void
  onComplete: (result: MutationResult) => void
}

export function MutationDialog({
  intent,
  agentID,
  rootID,
  allowReplace,
  onOpenChange,
  onComplete,
}: MutationDialogProps) {
  const [destination, setDestination] = useState("")
  const [conflict, setConflict] = useState<ConflictStrategy>("fail")
  const [appendText, setAppendText] = useState("")
  const [truncateSize, setTruncateSize] = useState("0")
  const [review, setReview] = useState<MutationResult>()
  const [result, setResult] = useState<MutationResult>()
  const [pending, setPending] = useState(false)
  const [error, setError] = useState("")
  const controllerRef = useRef<AbortController | undefined>(undefined)
  const items = useMemo(
    () =>
      intent
        ? mutationItems(intent, destination, appendText, truncateSize)
        : [],
    [appendText, destination, intent, truncateSize]
  )

  useEffect(() => {
    controllerRef.current?.abort()
    queueMicrotask(() => {
      setDestination(intent ? initialDestination(intent) : "")
      setConflict("fail")
      setAppendText("")
      setTruncateSize("0")
      setReview(undefined)
      setResult(undefined)
      setPending(false)
      setError("")
    })
  }, [intent])

  const validationError = intent
    ? validateIntent(intent, destination, appendText, truncateSize)
    : ""

  const run = async (dryRun: boolean) => {
    if (!intent || validationError || pending) return
    const controller = new AbortController()
    controllerRef.current = controller
    setPending(true)
    setError("")
    try {
      const nextResult = await mutateFiles(
        agentID,
        rootID,
        intent.verb,
        items,
        conflict,
        dryRun,
        controller.signal
      )
      if (dryRun) {
        setReview(nextResult)
      } else {
        setResult(nextResult)
        onComplete(nextResult)
      }
    } catch (cause) {
      if (!isAbortError(cause)) setError(errorMessage(cause))
    } finally {
      setPending(false)
      controllerRef.current = undefined
    }
  }

  const close = () => {
    if (!pending) onOpenChange(false)
  }
  const planSucceeded = review?.items.every((item) => item.state === "planned")
  const deletesDirectories =
    intent?.verb === "delete" &&
    intent.entries.some(({ entry }) => entry.kind === "directory")

  return (
    <Dialog
      open={intent !== undefined}
      onOpenChange={(open) => !open && close()}
    >
      <DialogContent showCloseButton={!pending} className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {intent ? actionTitle(intent) : "File operation"}
          </DialogTitle>
          <DialogDescription>
            {intent ? actionDescription(intent) : "Review this file operation."}
          </DialogDescription>
        </DialogHeader>
        {intent ? (
          <div className="flex flex-col gap-5">
            {deletesDirectories && !result ? (
              <Alert variant="destructive">
                <TriangleAlert />
                <AlertTitle>Folder contents will also be deleted</AlertTitle>
                <AlertDescription>
                  Every nested file and folder is permanently removed. Nested
                  contents are checked by the agent but are not individually
                  listed in this review.
                </AlertDescription>
              </Alert>
            ) : null}
            {!review && !result ? (
              <MutationFields
                intent={intent}
                destination={destination}
                conflict={conflict}
                appendText={appendText}
                truncateSize={truncateSize}
                error={validationError}
                disabled={pending}
                allowReplace={allowReplace}
                onDestinationChange={setDestination}
                onConflictChange={setConflict}
                onAppendTextChange={setAppendText}
                onTruncateSizeChange={setTruncateSize}
              />
            ) : null}
            {review && !result ? (
              <ResultList
                result={review}
                entries={intent.entries}
                heading="Dry-run review"
              />
            ) : null}
            {result ? (
              <ResultList
                result={result}
                entries={intent.entries}
                heading="Operation results"
              />
            ) : null}
            {error ? (
              <p className="text-sm text-destructive" role="alert">
                {error}
              </p>
            ) : null}
            <DialogFooter>
              {result ? (
                <Button onClick={close}>Done</Button>
              ) : review ? (
                <>
                  <Button
                    variant="outline"
                    disabled={pending}
                    onClick={() => setReview(undefined)}
                  >
                    Back
                  </Button>
                  <Button
                    variant={
                      intent.verb === "delete" ? "destructive" : "default"
                    }
                    disabled={pending || !planSucceeded}
                    onClick={() => void run(false)}
                  >
                    {pending ? (
                      <Spinner data-icon="inline-start" />
                    ) : intent.verb === "delete" ? (
                      <Trash2 data-icon="inline-start" />
                    ) : (
                      <FilePenLine data-icon="inline-start" />
                    )}
                    {pending ? "Applying…" : applyLabel(intent)}
                  </Button>
                </>
              ) : (
                <>
                  <Button variant="outline" disabled={pending} onClick={close}>
                    Cancel
                  </Button>
                  <Button
                    disabled={pending || validationError !== ""}
                    onClick={() => void run(true)}
                  >
                    {pending ? <Spinner data-icon="inline-start" /> : null}
                    {pending ? "Checking…" : "Review changes"}
                  </Button>
                </>
              )}
            </DialogFooter>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

function MutationFields({
  intent,
  destination,
  conflict,
  appendText,
  truncateSize,
  error,
  disabled,
  allowReplace,
  onDestinationChange,
  onConflictChange,
  onAppendTextChange,
  onTruncateSizeChange,
}: {
  intent: MutationIntent
  destination: string
  conflict: ConflictStrategy
  appendText: string
  truncateSize: string
  error: string
  disabled: boolean
  allowReplace: boolean
  onDestinationChange: (value: string) => void
  onConflictChange: (value: ConflictStrategy) => void
  onAppendTextChange: (value: string) => void
  onTruncateSizeChange: (value: string) => void
}) {
  const needsDestination = destinationVerbs.has(intent.verb)
  const hasConflictChoice = conflictVerbs.has(intent.verb)
  const availableConflictOptions = allowReplace
    ? conflictOptions
    : conflictOptions.filter((option) => option.value !== "replace")
  return (
    <FieldGroup>
      {intent.entries.length > 0 ? (
        <Field>
          <FieldLabel>
            {intent.entries.length === 1 ? "Source" : "Sources"}
          </FieldLabel>
          <div className="max-h-28 overflow-auto rounded-lg border p-2">
            {intent.entries.map(({ path }) => (
              <p key={path} className="truncate text-sm" title={path}>
                {path}
              </p>
            ))}
          </div>
        </Field>
      ) : null}
      {needsDestination ? (
        <Field data-invalid={error !== ""}>
          <FieldLabel htmlFor="mutation-destination">Destination</FieldLabel>
          <Input
            id="mutation-destination"
            value={destination}
            disabled={disabled}
            aria-invalid={error !== ""}
            onChange={(event) => onDestinationChange(event.target.value)}
          />
          <FieldDescription>
            Path relative to the selected root.
          </FieldDescription>
          <FieldError>{error}</FieldError>
        </Field>
      ) : null}
      {intent.verb === "append" ? (
        <Field data-invalid={error !== ""}>
          <FieldLabel htmlFor="mutation-append">Text to append</FieldLabel>
          <Textarea
            id="mutation-append"
            value={appendText}
            disabled={disabled}
            aria-invalid={error !== ""}
            onChange={(event) => onAppendTextChange(event.target.value)}
          />
          <FieldDescription>UTF-8 text, limited to 1 MiB.</FieldDescription>
          <FieldError>{error}</FieldError>
        </Field>
      ) : null}
      {intent.verb === "truncate" ? (
        <Field data-invalid={error !== ""}>
          <FieldLabel htmlFor="mutation-size">New size in bytes</FieldLabel>
          <Input
            id="mutation-size"
            type="number"
            min="0"
            step="1"
            value={truncateSize}
            disabled={disabled}
            aria-invalid={error !== ""}
            onChange={(event) => onTruncateSizeChange(event.target.value)}
          />
          <FieldDescription>
            Reducing the size permanently discards bytes beyond this point.
          </FieldDescription>
          <FieldError>{error}</FieldError>
        </Field>
      ) : null}
      {hasConflictChoice ? (
        <Field>
          <FieldLabel>When the destination exists</FieldLabel>
          <Select
            items={availableConflictOptions}
            value={conflict}
            disabled={disabled}
            onValueChange={(value) => {
              if (value) onConflictChange(value as ConflictStrategy)
            }}
          >
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                {availableConflictOptions.map((option) => (
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
  )
}

function ResultList({
  result,
  entries,
  heading,
}: {
  result: MutationResult
  entries: LocatedEntry[]
  heading: string
}) {
  return (
    <section className="flex flex-col gap-3" aria-label={heading}>
      <div className="flex items-center justify-between gap-3">
        <h3 className="font-medium">{heading}</h3>
        <Badge variant="outline">{result.items.length} items</Badge>
      </div>
      <div className="max-h-64 overflow-auto rounded-lg border">
        {result.items.map((item, index) => {
          const failed = item.state === "failed"
          const path = entries[index]?.path ?? item.result_path ?? item.item_id
          return (
            <div
              key={item.item_id}
              className="flex items-start justify-between gap-3 border-b p-3 last:border-b-0"
            >
              <div className="min-w-0">
                <p className="truncate text-sm font-medium" title={path}>
                  {path}
                </p>
                <p className="text-xs text-muted-foreground">
                  {failed
                    ? (item.error_class ?? "Operation rejected")
                    : item.state}
                </p>
              </div>
              {failed ? (
                <TriangleAlert
                  className="text-destructive"
                  aria-label="Failed"
                />
              ) : (
                <CheckCircle2
                  className="text-muted-foreground"
                  aria-label="Ready"
                />
              )}
            </div>
          )
        })}
      </div>
    </section>
  )
}

const destinationVerbs = new Set<MutationVerb>([
  "create_file",
  "create_directory",
  "rename",
  "move",
  "copy",
  "duplicate",
])

const conflictVerbs = new Set<MutationVerb>([
  "rename",
  "move",
  "copy",
  "duplicate",
])

function mutationItems(
  intent: MutationIntent,
  destination: string,
  appendText: string,
  truncateSize: string
): MutationItem[] {
  if (intent.entries.length === 0) {
    return [
      {
        item_id: crypto.randomUUID(),
        destination_path: destination,
        preconditions: {},
      },
    ]
  }
  return intent.entries.map(({ entry, path }, index) => ({
    item_id: `${entry.entry_id}-${index}`,
    source_path: path,
    destination_path: destinationVerbs.has(intent.verb)
      ? destination
      : undefined,
    append_data:
      intent.verb === "append" ? encodeBase64(appendText) : undefined,
    truncate_size:
      intent.verb === "truncate" ? Number(truncateSize) : undefined,
    preconditions: {
      must_exist: true,
      expected_kind: entry.kind,
      expected_size: entry.kind === "file" ? entry.size : undefined,
      expected_modified_at: entry.modified_at,
    },
  }))
}

function initialDestination(intent: MutationIntent) {
  if (intent.verb === "create_file")
    return joinPath(intent.directory, "new-file.txt")
  if (intent.verb === "create_directory")
    return joinPath(intent.directory, "new-folder")
  const source = intent.entries[0]?.path ?? ""
  if (intent.verb === "duplicate") return `${source} copy`
  return source
}

function validateIntent(
  intent: MutationIntent,
  destination: string,
  appendText: string,
  truncateSize: string
) {
  if (destinationVerbs.has(intent.verb) && !isValidRelativePath(destination)) {
    return "Use a normalized root-relative path without . or .. segments."
  }
  if (intent.verb === "append") {
    const size = new TextEncoder().encode(appendText).byteLength
    if (size === 0) return "Enter text to append."
    if (size > 1024 * 1024) return "Appended text cannot exceed 1 MiB."
  }
  if (intent.verb === "truncate") {
    const size = Number(truncateSize)
    if (!Number.isSafeInteger(size) || size < 0) {
      return "Enter a non-negative whole number of bytes."
    }
  }
  return ""
}

function encodeBase64(value: string) {
  const bytes = new TextEncoder().encode(value)
  let binary = ""
  const encodingChunkSize = 32 * 1024
  for (let offset = 0; offset < bytes.length; offset += encodingChunkSize) {
    binary += String.fromCharCode(
      ...bytes.subarray(offset, offset + encodingChunkSize)
    )
  }
  return btoa(binary)
}

function actionTitle(intent: MutationIntent) {
  const count = intent.entries.length
  switch (intent.verb) {
    case "create_file":
      return "Create file"
    case "create_directory":
      return "Create folder"
    case "rename":
    case "move":
      return "Move or rename"
    case "copy":
      return "Copy file"
    case "duplicate":
      return "Duplicate file"
    case "touch":
      return "Update modified time"
    case "truncate":
      return "Change file size"
    case "append":
      return "Append text"
    case "delete":
      return `Delete ${count} ${count === 1 ? "item" : "items"} permanently`
  }
}

function actionDescription(intent: MutationIntent) {
  if (intent.verb === "delete") {
    return "This cannot be undone. Selected files and folders are deleted immediately after the agent validates their current preconditions."
  }
  return "The agent will validate current file preconditions during a dry run before any change is applied."
}

function applyLabel(intent: MutationIntent) {
  return intent.verb === "delete" ? "Delete permanently" : "Apply changes"
}
