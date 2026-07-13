import { useState } from "react"
import { Download, RotateCw, Trash2, X } from "lucide-react"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { buttonVariants } from "@/components/ui/button-variants"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Progress } from "@/components/ui/progress"
import {
  controlTransfer,
  removeFinishedTransfers,
  removeTransfer,
  transferContentURL,
  type FileTransfer,
} from "@/lib/files"

import { errorMessage, isTerminalTransfer } from "./shared"

type RemovalTarget =
  | { kind: "single"; transfer: FileTransfer }
  | { kind: "finished"; count: number }

type TransferDrawerProps = {
  agentID: string
  transfers: FileTransfer[]
  error: string
  onChange: (transfer: FileTransfer) => void
  onRemove: (transferID: string) => void
  onRemoveFinished: () => void
}

export function TransferDrawer({
  agentID,
  transfers,
  error,
  onChange,
  onRemove,
  onRemoveFinished,
}: TransferDrawerProps) {
  const [pendingAction, setPendingAction] = useState("")
  const [actionError, setActionError] = useState("")
  const [removalTarget, setRemovalTarget] = useState<RemovalTarget>()
  const finishedCount = transfers.filter(isTerminalTransfer).length
  const actionPending = pendingAction !== ""

  const requestRemoval = (target: RemovalTarget) => {
    setActionError("")
    setRemovalTarget(target)
  }

  const control = async (
    transfer: FileTransfer,
    action: "resume" | "abort"
  ) => {
    if (actionPending) return
    setPendingAction(`${action}:${transfer.transfer_id}`)
    setActionError("")
    try {
      onChange(await controlTransfer(agentID, transfer.transfer_id, action))
    } catch (cause) {
      setActionError(errorMessage(cause))
    } finally {
      setPendingAction("")
    }
  }

  const confirmRemoval = async () => {
    if (!removalTarget || actionPending) return
    setPendingAction("remove")
    setActionError("")
    try {
      if (removalTarget.kind === "single") {
        await removeTransfer(agentID, removalTarget.transfer.transfer_id)
        onRemove(removalTarget.transfer.transfer_id)
      } else {
        await removeFinishedTransfers(agentID)
        onRemoveFinished()
      }
      setRemovalTarget(undefined)
    } catch (cause) {
      setActionError(errorMessage(cause))
    } finally {
      setPendingAction("")
    }
  }

  return (
    <>
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between gap-2">
            <CardTitle>Transfers</CardTitle>
            {finishedCount > 1 ? (
              <Button
                size="icon"
                aria-label={`Remove ${finishedCount} finished transfers`}
                variant="destructive"
                disabled={actionPending}
                onClick={() =>
                  requestRemoval({ kind: "finished", count: finishedCount })
                }
              >
                <Trash2 data-icon="inline-start" />
              </Button>
            ) : null}
          </div>
          <CardDescription>
            Uploads are published atomically after verification. Completed
            downloads remain staged until Save to computer is selected.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          {error || actionError ? (
            <p className="text-sm text-destructive" role="alert">
              {error || actionError}
            </p>
          ) : null}
          {transfers.length === 0 ? (
            <p className="text-sm text-muted-foreground">No transfers yet.</p>
          ) : (
            <>
              {transfers.slice(0, 10).map((transfer) => (
                <TransferItem
                  key={transfer.transfer_id}
                  agentID={agentID}
                  transfer={transfer}
                  actionPending={actionPending}
                  onControl={control}
                  onRemove={() => requestRemoval({ kind: "single", transfer })}
                />
              ))}
              {transfers.length > 10 ? (
                <p className="text-xs text-muted-foreground">
                  Showing the 10 newest of {transfers.length} transfers.
                </p>
              ) : null}
            </>
          )}
        </CardContent>
      </Card>
      <RemovalConfirmation
        target={removalTarget}
        pending={pendingAction === "remove"}
        error={actionError}
        onOpenChange={(open) => {
          if (!open && pendingAction !== "remove") {
            setRemovalTarget(undefined)
          }
        }}
        onConfirm={() => void confirmRemoval()}
      />
    </>
  )
}

function TransferItem({
  agentID,
  transfer,
  actionPending,
  onControl,
  onRemove,
}: {
  agentID: string
  transfer: FileTransfer
  actionPending: boolean
  onControl: (
    transfer: FileTransfer,
    action: "resume" | "abort"
  ) => Promise<void>
  onRemove: () => void
}) {
  const progress = transferProgress(transfer)

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">
            {transfer.manifest.relative_path}
          </p>
          <p className="text-xs text-muted-foreground">
            {transfer.manifest.direction} · {transferStateLabel(transfer)}
          </p>
        </div>
        <Badge variant="secondary">{Math.round(progress)}%</Badge>
      </div>
      <Progress
        value={progress}
        aria-label={`Transfer ${Math.round(progress)} percent complete`}
      />
      <div className="flex flex-wrap gap-2">
        {transfer.state === "completed" &&
        transfer.manifest.direction === "download" ? (
          <a
            className={buttonVariants({ size: "xs", variant: "outline" })}
            href={transferContentURL(agentID, transfer.transfer_id)}
          >
            <Download data-icon="inline-start" /> Save to computer
          </a>
        ) : null}
        {transfer.state === "paused" ? (
          <Button
            size="xs"
            variant="outline"
            disabled={actionPending}
            onClick={() => void onControl(transfer, "resume")}
          >
            <RotateCw data-icon="inline-start" /> Resume
          </Button>
        ) : null}
        {!isTerminalTransfer(transfer) ? (
          <Button
            size="xs"
            variant="ghost"
            disabled={actionPending}
            onClick={() => void onControl(transfer, "abort")}
          >
            <X data-icon="inline-start" /> Cancel
          </Button>
        ) : null}
        {isTerminalTransfer(transfer) ? (
          <Button
            size="xs"
            className="ml-auto"
            variant="destructive"
            disabled={actionPending}
            onClick={onRemove}
          >
            <Trash2 data-icon="inline-start" /> Remove
          </Button>
        ) : null}
      </div>
    </div>
  )
}

function RemovalConfirmation({
  target,
  pending,
  error,
  onOpenChange,
  onConfirm,
}: {
  target?: RemovalTarget
  pending: boolean
  error: string
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}) {
  const count = target?.kind === "finished" ? target.count : 1
  const title =
    target?.kind === "finished"
      ? `Remove ${count} finished transfers?`
      : "Remove this transfer?"

  return (
    <AlertDialog open={target !== undefined} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogMedia>
            <Trash2 />
          </AlertDialogMedia>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>
            This permanently removes the gateway transfer record and encrypted
            staging data. The source file on the agent and files already saved
            to this computer are not affected.
          </AlertDialogDescription>
          {error ? (
            <p className="text-sm text-destructive" role="alert">
              {error}
            </p>
          ) : null}
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>Cancel</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={pending}
            onClick={onConfirm}
          >
            <Trash2 data-icon="inline-start" />
            {pending ? "Removing…" : count === 1 ? "Remove" : "Remove finished"}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

function transferProgress(transfer: FileTransfer) {
  if (transfer.state === "completed") return 100
  if (transfer.manifest.size > 0) {
    const progress = (transfer.bytes_verified / transfer.manifest.size) * 100
    return Number.isFinite(progress) ? Math.min(100, Math.max(0, progress)) : 0
  }
  return 0
}

function transferStateLabel(transfer: FileTransfer) {
  if (
    transfer.state === "completed" &&
    transfer.manifest.direction === "download"
  ) {
    return "ready to save"
  }
  return transfer.state
}
