import {
  Archive,
  Clock3,
  Copy,
  Ellipsis,
  FilePenLine,
  FolderInput,
  ListTree,
  Move,
  ScissorsLineDashed,
  Trash2,
} from "lucide-react"
import { useRef, useState } from "react"

import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import type { FileEntry } from "@/lib/files"

import type { MutationVerb } from "@/lib/files"

import { FileEntryIcon, PermissionDisplay } from "./entry-presentation"
import { formatBytes, formatModifiedAt } from "./shared"

export function DirectoryTable({
  entries,
  selectedEntryIDs,
  canMutate,
  canDelete,
  canSelect,
  canArchive,
  onOpen,
  onSelectionChange,
  onSelectionRange,
  onAction,
  onArchiveAction,
}: {
  entries: FileEntry[]
  selectedEntryIDs: Set<string>
  canMutate: boolean
  canDelete: boolean
  canSelect: boolean
  canArchive: boolean
  onOpen: (entry: FileEntry) => void
  onSelectionChange: (entry: FileEntry, selected: boolean) => void
  onSelectionRange: (entries: FileEntry[], selected: boolean) => void
  onAction: (verb: MutationVerb, entry: FileEntry) => void
  onArchiveAction: (action: "list" | "extract", entry: FileEntry) => void
}) {
  const rowHeight = 60
  const viewportHeight = 520
  const overscan = 6
  const [scrollTop, setScrollTop] = useState(0)
  const [selectionAnchor, setSelectionAnchor] = useState<number>()
  const shiftSelectionRef = useRef(false)
  const startIndex = Math.max(0, Math.floor(scrollTop / rowHeight) - overscan)
  const endIndex = Math.min(
    entries.length,
    startIndex + Math.ceil(viewportHeight / rowHeight) + overscan * 2
  )
  const visibleEntries = entries.slice(startIndex, endIndex)
  const paddingTop = startIndex * rowHeight
  const paddingBottom = (entries.length - endIndex) * rowHeight
  const selectableEntries = entries.filter((entry) => entry.operation_name)
  const allSelected =
    selectableEntries.length > 0 &&
    selectableEntries.every((entry) => selectedEntryIDs.has(entry.entry_id))

  return (
    <div
      className="max-h-130 overflow-auto rounded-lg border"
      onScroll={(event) => setScrollTop(event.currentTarget.scrollTop)}
    >
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-10">
              <Checkbox
                aria-label="Select all visible entries"
                checked={allSelected}
                disabled={!canSelect || selectableEntries.length === 0}
                onCheckedChange={(checked) => {
                  for (const entry of selectableEntries) {
                    onSelectionChange(entry, checked)
                  }
                }}
              />
            </TableHead>
            <TableHead>Name</TableHead>
            <TableHead className="w-28">Permissions</TableHead>
            <TableHead>Size</TableHead>
            <TableHead>Modified</TableHead>
            <TableHead className="w-10">
              <span className="sr-only">Actions</span>
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {paddingTop > 0 ? (
            <TableRow aria-hidden="true">
              <TableCell
                colSpan={6}
                className="p-0"
                style={{ height: paddingTop }}
              />
            </TableRow>
          ) : null}
          {visibleEntries.map((entry, visibleIndex) => {
            const entryIndex = startIndex + visibleIndex
            const isDirectory = entry.kind === "directory"
            return (
              <TableRow
                key={entry.entry_id}
                data-index={entryIndex}
                className="group/entry hover:bg-muted/50"
              >
                <TableCell className="w-10">
                  <Checkbox
                    aria-label={`Select ${entry.display_name}`}
                    checked={selectedEntryIDs.has(entry.entry_id)}
                    disabled={!canSelect || !entry.operation_name}
                    onClick={(event) => {
                      shiftSelectionRef.current = event.shiftKey
                    }}
                    onCheckedChange={(checked) => {
                      if (
                        shiftSelectionRef.current &&
                        selectionAnchor !== undefined
                      ) {
                        const first = Math.min(selectionAnchor, entryIndex)
                        const last = Math.max(selectionAnchor, entryIndex)
                        onSelectionRange(
                          entries
                            .slice(first, last + 1)
                            .filter((candidate) => candidate.operation_name),
                          checked
                        )
                      } else {
                        onSelectionChange(entry, checked)
                      }
                      shiftSelectionRef.current = false
                      setSelectionAnchor(entryIndex)
                    }}
                  />
                </TableCell>
                <TableCell>
                  <Button
                    variant="ghost"
                    className="h-9 max-w-105 justify-start px-2 hover:bg-transparent"
                    disabled={!entry.operation_name}
                    aria-label={`Open ${entry.kind} ${entry.display_name}`}
                    title={
                      entry.operation_name
                        ? undefined
                        : "This native name cannot be addressed safely through the normalized path protocol"
                    }
                    onClick={() => onOpen(entry)}
                  >
                    <span
                      className="flex size-6 shrink-0 items-center justify-center rounded-md bg-muted text-foreground"
                      aria-hidden="true"
                    >
                      <FileEntryIcon kind={entry.kind} />
                    </span>
                    <span className="truncate">{entry.display_name}</span>
                  </Button>
                </TableCell>
                <TableCell className="w-28">
                  <PermissionDisplay mode={entry.mode} />
                </TableCell>
                <TableCell>
                  {isDirectory ? "—" : formatBytes(entry.size)}
                </TableCell>
                <TableCell>{formatModifiedAt(entry.modified_at)}</TableCell>
                <TableCell>
                  {(canMutate || canArchive) && entry.operation_name ? (
                    <EntryActions
                      entry={entry}
                      canMutate={canMutate}
                      canDelete={canDelete}
                      canArchive={canArchive}
                      onAction={onAction}
                      onArchiveAction={onArchiveAction}
                    />
                  ) : null}
                </TableCell>
              </TableRow>
            )
          })}
          {paddingBottom > 0 ? (
            <TableRow aria-hidden="true">
              <TableCell
                colSpan={6}
                className="p-0"
                style={{ height: paddingBottom }}
              />
            </TableRow>
          ) : null}
        </TableBody>
      </Table>
    </div>
  )
}

function EntryActions({
  entry,
  canMutate,
  canDelete,
  canArchive,
  onAction,
  onArchiveAction,
}: {
  entry: FileEntry
  canMutate: boolean
  canDelete: boolean
  canArchive: boolean
  onAction: (verb: MutationVerb, entry: FileEntry) => void
  onArchiveAction: (action: "list" | "extract", entry: FileEntry) => void
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label={`Actions for ${entry.display_name}`}
          />
        }
      >
        <Ellipsis />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-48">
        {canMutate ? (
          <DropdownMenuGroup>
            <DropdownMenuItem onClick={() => onAction("move", entry)}>
              <Move /> Move or rename
            </DropdownMenuItem>
            {entry.kind === "file" ? (
              <>
                <DropdownMenuItem onClick={() => onAction("copy", entry)}>
                  <Copy /> Copy
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => onAction("duplicate", entry)}>
                  <FilePenLine /> Duplicate
                </DropdownMenuItem>
              </>
            ) : null}
            <DropdownMenuItem onClick={() => onAction("touch", entry)}>
              <Clock3 /> Update modified time
            </DropdownMenuItem>
            {entry.kind === "file" ? (
              <>
                <DropdownMenuItem onClick={() => onAction("append", entry)}>
                  <FilePenLine /> Append text
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => onAction("truncate", entry)}>
                  <ScissorsLineDashed /> Change file size
                </DropdownMenuItem>
              </>
            ) : null}
          </DropdownMenuGroup>
        ) : null}
        {canArchive && isZIPEntry(entry) ? (
          <>
            {canMutate ? <DropdownMenuSeparator /> : null}
            <DropdownMenuGroup>
              <DropdownMenuItem onClick={() => onArchiveAction("list", entry)}>
                <ListTree /> Inspect archive
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => onArchiveAction("extract", entry)}
              >
                <FolderInput /> Extract safely
              </DropdownMenuItem>
            </DropdownMenuGroup>
          </>
        ) : null}
        {canArchive && entry.kind !== "file" ? (
          <DropdownMenuGroup>
            <DropdownMenuItem disabled>
              <Archive /> Select to add to archive
            </DropdownMenuItem>
          </DropdownMenuGroup>
        ) : null}
        {canDelete ? (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuItem
                variant="destructive"
                onClick={() => onAction("delete", entry)}
              >
                <Trash2 /> Delete permanently
              </DropdownMenuItem>
            </DropdownMenuGroup>
          </>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function isZIPEntry(entry: FileEntry) {
  return (
    entry.kind === "file" &&
    entry.display_name.toLocaleLowerCase().endsWith(".zip")
  )
}
