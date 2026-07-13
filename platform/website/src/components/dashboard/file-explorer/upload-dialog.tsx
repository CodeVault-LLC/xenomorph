import { useEffect, useRef, useState } from "react"
import { Upload } from "lucide-react"

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
import { Progress } from "@/components/ui/progress"
import { Spinner } from "@/components/ui/spinner"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  uploadFile,
  type ConflictStrategy,
  type FileTransfer,
  type UploadProgress,
} from "@/lib/files"

import {
  errorMessage,
  isAbortError,
  isValidRelativePath,
  joinPath,
} from "./shared"

const conflictOptions = [
  { label: "Stop if a file exists", value: "fail" },
  { label: "Keep both files", value: "rename_new" },
  { label: "Replace the existing file", value: "replace" },
] as const

type UploadDialogProps = {
  open: boolean
  agentID: string
  rootID: string
  directory: string
  allowReplace: boolean
  onOpenChange: (open: boolean) => void
  onTransfer: (transfer: FileTransfer) => void
  onComplete: () => void
}

export function UploadDialog({
  open,
  agentID,
  rootID,
  directory,
  allowReplace,
  onOpenChange,
  onTransfer,
  onComplete,
}: UploadDialogProps) {
  const [file, setFile] = useState<File>()
  const [destination, setDestination] = useState("")
  const [conflict, setConflict] = useState<ConflictStrategy>("fail")
  const [progress, setProgress] = useState<UploadProgress>()
  const [error, setError] = useState("")
  const controllerRef = useRef<AbortController | undefined>(undefined)
  const pending = progress !== undefined
  const availableConflictOptions = allowReplace
    ? conflictOptions
    : conflictOptions.filter((option) => option.value !== "replace")

  useEffect(() => {
    if (open) return
    controllerRef.current?.abort()
    queueMicrotask(() => {
      setFile(undefined)
      setDestination("")
      setConflict("fail")
      setProgress(undefined)
      setError("")
    })
  }, [open])

  useEffect(() => () => controllerRef.current?.abort(), [])

  const chooseFile = (nextFile?: File) => {
    setFile(nextFile)
    setDestination(nextFile ? joinPath(directory, nextFile.name) : "")
    setError("")
  }

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!file || !isValidRelativePath(destination) || pending) return
    const controller = new AbortController()
    controllerRef.current = controller
    setProgress({ phase: "hashing", bytesComplete: 0, bytesTotal: file.size })
    setError("")
    try {
      await uploadFile(
        agentID,
        rootID,
        destination,
        file,
        conflict,
        (nextProgress) => {
          setProgress(nextProgress)
          if (nextProgress.transfer) onTransfer(nextProgress.transfer)
        },
        controller.signal
      )
      onComplete()
      onOpenChange(false)
    } catch (cause) {
      if (!isAbortError(cause)) setError(errorMessage(cause))
      setProgress(undefined)
    } finally {
      controllerRef.current = undefined
    }
  }

  const pathError =
    destination !== "" && !isValidRelativePath(destination)
      ? "Use a normalized root-relative path without . or .. segments."
      : ""
  const percentage = progress
    ? progress.bytesTotal === 0
      ? progress.phase === "hashing"
        ? 0
        : 100
      : Math.round((progress.bytesComplete / progress.bytesTotal) * 100)
    : 0

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!pending) onOpenChange(nextOpen)
      }}
    >
      <DialogContent showCloseButton={!pending}>
        <DialogHeader>
          <DialogTitle>Upload a file</DialogTitle>
          <DialogDescription>
            The browser checksums and stages one file at the gateway before the
            agent publishes it atomically in this root.
          </DialogDescription>
        </DialogHeader>
        <form className="flex flex-col gap-5" onSubmit={submit}>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="workspace-upload-file">File</FieldLabel>
              <Input
                id="workspace-upload-file"
                type="file"
                disabled={pending}
                onChange={(event) => chooseFile(event.target.files?.[0])}
              />
              <FieldDescription>
                Individual files may be up to 1 GiB.
              </FieldDescription>
            </Field>
            <Field data-invalid={pathError !== ""}>
              <FieldLabel htmlFor="workspace-upload-destination">
                Destination
              </FieldLabel>
              <Input
                id="workspace-upload-destination"
                value={destination}
                disabled={pending}
                aria-invalid={pathError !== ""}
                placeholder={joinPath(directory, "filename.ext")}
                onChange={(event) => setDestination(event.target.value)}
              />
              <FieldDescription>
                Path relative to the selected root.
              </FieldDescription>
              <FieldError>{pathError}</FieldError>
            </Field>
            <Field>
              <FieldLabel>When the destination exists</FieldLabel>
              <Select
                items={availableConflictOptions}
                value={conflict}
                disabled={pending}
                onValueChange={(value) => {
                  if (value) setConflict(value as ConflictStrategy)
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
          </FieldGroup>
          {progress ? (
            <div className="flex flex-col gap-2" role="status">
              <div className="flex justify-between gap-3 text-sm">
                <span>{uploadPhaseLabel(progress.phase)}</span>
                <span className="text-muted-foreground">{percentage}%</span>
              </div>
              <Progress
                value={percentage}
                aria-label={`${percentage}% uploaded`}
              />
            </div>
          ) : null}
          {error ? (
            <p className="text-sm text-destructive" role="alert">
              {error}
            </p>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={pending}
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={!file || !isValidRelativePath(destination) || pending}
            >
              {pending ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <Upload data-icon="inline-start" />
              )}
              {pending ? "Uploading…" : "Upload"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function uploadPhaseLabel(phase: UploadProgress["phase"]) {
  switch (phase) {
    case "hashing":
      return "Preparing checksums"
    case "staging":
      return "Staging encrypted chunks"
    case "committing":
      return "Publishing on agent"
  }
}
