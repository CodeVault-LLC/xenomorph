import { CheckCircle2, Info, Save } from "lucide-react"
import { useMemo, useState } from "react"

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
import { Spinner } from "@/components/ui/spinner"
import {
  setFileMetadata,
  type MetadataResult,
  type MetadataSetResult,
  type RootCapabilities,
} from "@/lib/files"

import { errorMessage } from "./shared"

export function MetadataDialog({
  open,
  agentID,
  rootID,
  path,
  metadata,
  capabilities,
  onOpenChange,
  onComplete,
}: {
  open: boolean
  agentID: string
  rootID: string
  path: string
  metadata: MetadataResult
  capabilities: RootCapabilities
  onOpenChange: (open: boolean) => void
  onComplete: () => void
}) {
  const initialModified = useMemo(
    () => toLocalDateTime(metadata.modified_at),
    [metadata.modified_at]
  )
  const initialMode = (metadata.mode & 0o7777).toString(8).padStart(3, "0")
  const [modified, setModified] = useState(initialModified)
  const [mode, setMode] = useState(initialMode)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState("")
  const [result, setResult] = useState<MetadataSetResult>()
  const [maximumModified] = useState(() => Date.now() + 24 * 60 * 60 * 1000)

  const modeAvailable = capabilities.posix_mode === "available"
  const writeAvailable = capabilities.metadata_write === "available"
  const modeValid =
    /^[0-7]{3,4}$/.test(mode) && Number.parseInt(mode, 8) <= 0o7777
  const modifiedTime = new Date(modified).getTime()
  const modifiedValid =
    modified !== "" &&
    Number.isFinite(modifiedTime) &&
    modifiedTime >= 0 &&
    modifiedTime <= maximumModified
  const modifiedChanged = modified !== initialModified
  const modeChanged = modeAvailable && mode !== initialMode
  const hasChanges = modifiedChanged || modeChanged

  const submit = async () => {
    if (!writeAvailable || !hasChanges || !modeValid || !modifiedValid) return
    const delta: { modified_at?: string; posix_mode?: number } = {}
    if (modifiedChanged) delta.modified_at = new Date(modified).toISOString()
    if (modeChanged) delta.posix_mode = Number.parseInt(mode, 8)
    setPending(true)
    setError("")
    setResult(undefined)
    try {
      const next = await setFileMetadata(
        agentID,
        rootID,
        path,
        {
          must_exist: true,
          expected_kind: metadata.kind,
          expected_size: metadata.size,
          expected_modified_at: metadata.modified_at,
        },
        delta
      )
      setResult(next)
      if (next.fields.some((field) => field.state === "applied")) onComplete()
    } catch (cause) {
      setError(errorMessage(cause))
    } finally {
      setPending(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit metadata</DialogTitle>
          <DialogDescription>
            Apply explicit metadata changes to {path}. The agent revalidates the
            selected entry before each native update.
          </DialogDescription>
        </DialogHeader>
        {!writeAvailable ? (
          <Alert>
            <Info />
            <AlertTitle>Metadata updates unavailable</AlertTitle>
            <AlertDescription>
              This agent reports metadata writes as{" "}
              {capabilities.metadata_write}. No field will be submitted.
            </AlertDescription>
          </Alert>
        ) : (
          <FieldGroup>
            <Field data-invalid={!modifiedValid}>
              <FieldLabel htmlFor="metadata-modified">Modified time</FieldLabel>
              <Input
                id="metadata-modified"
                type="datetime-local"
                step="1"
                value={modified}
                disabled={pending}
                aria-invalid={!modifiedValid}
                onChange={(event) => setModified(event.target.value)}
              />
              <FieldDescription>
                Stored in UTC after the local date and time is submitted.
              </FieldDescription>
              <FieldError>
                {!modifiedValid
                  ? "Choose a valid date from 1970 through 24 hours from now."
                  : ""}
              </FieldError>
            </Field>
            <Field data-invalid={modeAvailable && !modeValid}>
              <FieldLabel htmlFor="metadata-mode">POSIX mode</FieldLabel>
              <Input
                id="metadata-mode"
                inputMode="numeric"
                pattern="[0-7]{3,4}"
                maxLength={4}
                value={mode}
                disabled={pending || !modeAvailable}
                aria-invalid={modeAvailable && !modeValid}
                onChange={(event) => setMode(event.target.value)}
              />
              <FieldDescription>
                {modeAvailable
                  ? "Enter three or four octal digits. Owner and ACL changes are separate and are not inferred."
                  : `POSIX mode is ${capabilities.posix_mode} on this filesystem.`}
              </FieldDescription>
              <FieldError>
                {modeAvailable && !modeValid
                  ? "Enter a mode between 000 and 7777 using octal digits."
                  : ""}
              </FieldError>
            </Field>
          </FieldGroup>
        )}
        {error ? (
          <Alert variant="destructive">
            <AlertTitle>Metadata update failed</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        {result ? <MetadataOutcome result={result} /> : null}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {result ? "Close" : "Cancel"}
          </Button>
          <Button
            disabled={
              pending ||
              !writeAvailable ||
              !hasChanges ||
              !modeValid ||
              !modifiedValid ||
              !!result
            }
            onClick={() => void submit()}
          >
            {pending ? (
              <Spinner data-icon="inline-start" />
            ) : (
              <Save data-icon="inline-start" />
            )}
            {pending ? "Applying…" : "Apply changes"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function MetadataOutcome({ result }: { result: MetadataSetResult }) {
  const applied = result.fields.filter(
    (field) => field.state === "applied"
  ).length
  return (
    <Alert>
      <CheckCircle2 />
      <AlertTitle>
        {applied === result.fields.length
          ? "Metadata updated"
          : "Metadata partially updated"}
      </AlertTitle>
      <AlertDescription>
        {result.fields
          .map((field) => `${field.field}: ${field.state}`)
          .join(" · ")}
      </AlertDescription>
    </Alert>
  )
}

function toLocalDateTime(value: string) {
  const date = new Date(value)
  const offset = date.getTimezoneOffset() * 60_000
  return new Date(date.getTime() - offset).toISOString().slice(0, 19)
}
