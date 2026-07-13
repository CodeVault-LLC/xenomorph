import type { FileTransfer } from "@/lib/files"

const byteFormatter = new Intl.NumberFormat(undefined, {
  style: "unit",
  unit: "byte",
  notation: "compact",
  unitDisplay: "narrow",
})

const modifiedAtFormatter = new Intl.DateTimeFormat(undefined, {
  dateStyle: "medium",
  timeStyle: "short",
})

export function formatBytes(value: number) {
  return byteFormatter.format(value)
}

export function formatModifiedAt(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime())
    ? "Invalid timestamp"
    : modifiedAtFormatter.format(date)
}

export function errorMessage(cause: unknown) {
  return cause instanceof Error
    ? cause.message
    : "File workspace request failed"
}

export function isAbortError(cause: unknown) {
  return cause instanceof DOMException && cause.name === "AbortError"
}

export function isTerminalTransfer(transfer: FileTransfer) {
  return ["completed", "failed", "cancelled"].includes(transfer.state)
}

export function joinPath(parent: string, name: string) {
  return parent ? `${parent}/${name}` : name
}

export function isValidRelativePath(value: string) {
  if (
    !value ||
    value.length > 4096 ||
    value.includes("\\") ||
    value.includes("\0")
  ) {
    return false
  }
  const parts = value.split("/")
  return parts.every(
    (part) =>
      part !== "" && part !== "." && part !== ".." && part.length <= 255
  )
}
