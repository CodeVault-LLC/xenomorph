import {
  FileCog,
  FileSymlink,
  FileText,
  Folder,
  type LucideIcon,
} from "lucide-react"

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import type { FileEntryKind } from "@/lib/files"

type FileEntryIconProps = {
  kind: FileEntryKind
}

type PermissionDisplayProps = {
  mode: number
}

type PermissionSymbol = "r" | "w" | "x"

type PermissionBit = readonly [bit: number, symbol: PermissionSymbol]

type PermissionAction = readonly [bit: number, label: string]

type PermissionGroup = {
  label: "Owner" | "Group" | "Others"
  shift: number
}

const entryIcons = {
  directory: Folder,
  file: FileText,
  symlink: FileSymlink,
  special: FileCog,
} satisfies Record<FileEntryKind, LucideIcon>

const permissionBits = [
  [0o400, "r"],
  [0o200, "w"],
  [0o100, "x"],
  [0o040, "r"],
  [0o020, "w"],
  [0o010, "x"],
  [0o004, "r"],
  [0o002, "w"],
  [0o001, "x"],
] as const satisfies readonly PermissionBit[]

const permissionGroups = [
  { label: "Owner", shift: 6 },
  { label: "Group", shift: 3 },
  { label: "Others", shift: 0 },
] as const satisfies readonly PermissionGroup[]

const permissionActions = [
  [0b100, "read"],
  [0b010, "write"],
  [0b001, "execute"],
] as const satisfies readonly PermissionAction[]

export function FileEntryIcon({ kind }: FileEntryIconProps) {
  const Icon = entryIcons[kind]

  return <Icon className="size-4" />
}

export function PermissionDisplay({ mode }: PermissionDisplayProps) {
  const permissions = formatPermissions(mode)
  const octalPermissions = formatOctalPermissions(mode)
  const summary = permissionGroups
    .map(
      ({ label, shift }) =>
        `${label.toLowerCase()} ${formatPermissionGroup(mode, shift)}`
    )
    .join("; ")

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <code
            className="text-xs text-muted-foreground"
            tabIndex={0}
            aria-label={`Permissions ${permissions}: ${summary}`}
          >
            {permissions}
          </code>
        }
      />
      <TooltipContent className="flex max-w-64 flex-col items-start gap-1.5 py-2">
        <span className="font-mono">{permissions}</span>
        {permissionGroups.map(({ label, shift }) => (
          <span key={label}>
            {label}: {formatPermissionGroup(mode, shift)}
          </span>
        ))}
        <span className="text-background/70">Octal: {octalPermissions}</span>
      </TooltipContent>
    </Tooltip>
  )
}

function formatPermissions(mode: number) {
  return permissionBits
    .map(([bit, symbol]) => (mode & bit ? symbol : "-"))
    .join("")
}

function formatOctalPermissions(mode: number) {
  return (mode & 0o777).toString(8).padStart(3, "0")
}

function formatPermissionGroup(mode: number, shift: number) {
  const group = (mode >> shift) & 0b111
  const values = permissionActions
    .filter(([bit]) => Boolean(group & bit))
    .map(([, label]) => label)

  return values.length > 0 ? values.join(", ") : "no access"
}
