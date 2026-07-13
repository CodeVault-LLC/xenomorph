import {
  ChevronLeft,
  ChevronRight,
  FilePlus2,
  FileQuestion,
  FolderOpen,
  FolderPlus,
  HardDrive,
  Trash2,
  Upload,
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
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { Skeleton } from "@/components/ui/skeleton"
import type {
  DirectoryPage,
  FileEntry,
  FileRoot,
  MutationVerb,
} from "@/lib/files"

import { DirectoryTable } from "./directory-table"
import { PathNavigation } from "./path-navigation"

type FileBrowserCardProps = {
  roots: FileRoot[]
  root?: FileRoot
  relativePath: string
  page?: DirectoryPage
  loading: boolean
  error: string
  canTransfer: boolean
  canMutate: boolean
  canDelete: boolean
  selectedEntryIDs: Set<string>
  cursorIndex: number
  onUpload: () => void
  onCreate: (verb: "create_file" | "create_directory") => void
  onBulkDelete: () => void
  onRootChange: (root: FileRoot) => void
  onNavigate: (path: string) => void
  onOpen: (entry: FileEntry) => void
  onSelectionChange: (entry: FileEntry, selected: boolean) => void
  onAction: (verb: MutationVerb, entry: FileEntry) => void
  onPreviousPage: () => void
  onNextPage: () => void
}

export function FileBrowserCard({
  roots,
  root,
  relativePath,
  page,
  loading,
  error,
  canTransfer,
  canMutate,
  canDelete,
  selectedEntryIDs,
  cursorIndex,
  onUpload,
  onCreate,
  onBulkDelete,
  onRootChange,
  onNavigate,
  onOpen,
  onSelectionChange,
  onAction,
  onPreviousPage,
  onNextPage,
}: FileBrowserCardProps) {
  return (
    <Card className="min-w-0">
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="flex flex-col gap-1">
            <CardTitle>Files</CardTitle>
            <CardDescription>
              Browse and perform gateway-mediated file operations on this agent.
            </CardDescription>
          </div>
          <div className="flex flex-wrap gap-2">
            {canDelete && selectedEntryIDs.size > 0 ? (
              <Button size="sm" variant="destructive" onClick={onBulkDelete}>
                <Trash2 data-icon="inline-start" /> Delete{" "}
                {selectedEntryIDs.size} permanently
              </Button>
            ) : null}
            <Button
              variant="outline"
              size="sm"
              disabled={!canTransfer}
              onClick={onUpload}
            >
              <Upload data-icon="inline-start" /> Upload
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger
                disabled={!canMutate}
                render={<Button variant="outline" size="sm" />}
              >
                <FilePlus2 data-icon="inline-start" /> New
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuGroup>
                  <DropdownMenuItem onClick={() => onCreate("create_file")}>
                    <FilePlus2 /> File
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={() => onCreate("create_directory")}
                  >
                    <FolderPlus /> Folder
                  </DropdownMenuItem>
                </DropdownMenuGroup>
              </DropdownMenuContent>
            </DropdownMenu>
            <Badge variant="outline">No-follow workspace</Badge>
          </div>
        </div>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <RootSelector
          roots={roots}
          selectedRoot={root}
          onChange={onRootChange}
        />
        <PathNavigation relativePath={relativePath} onNavigate={onNavigate} />
        <DirectoryContent
          error={error}
          loading={loading}
          root={root}
          page={page}
          selectedEntryIDs={selectedEntryIDs}
          canMutate={canMutate}
          canDelete={canDelete}
          onOpen={onOpen}
          onSelectionChange={onSelectionChange}
          onAction={onAction}
        />
        <DirectoryPagination
          page={page}
          cursorIndex={cursorIndex}
          loading={loading}
          onPrevious={onPreviousPage}
          onNext={onNextPage}
        />
      </CardContent>
    </Card>
  )
}

function RootSelector({
  roots,
  selectedRoot,
  onChange,
}: {
  roots: FileRoot[]
  selectedRoot?: FileRoot
  onChange: (root: FileRoot) => void
}) {
  return (
    <div className="flex flex-wrap gap-2" aria-label="Filesystem roots">
      {roots.map((candidate) => {
        return (
          <Button
            key={candidate.root_id}
            variant={
              candidate.root_id === selectedRoot?.root_id
                ? "secondary"
                : "outline"
            }
            size="sm"
            aria-pressed={candidate.root_id === selectedRoot?.root_id}
            onClick={() => onChange(candidate)}
          >
            <HardDrive data-icon="inline-start" />
            {candidate.display_label}
          </Button>
        )
      })}
    </div>
  )
}

function DirectoryContent({
  error,
  loading,
  root,
  page,
  selectedEntryIDs,
  canMutate,
  canDelete,
  onOpen,
  onSelectionChange,
  onAction,
}: {
  error: string
  loading: boolean
  root?: FileRoot
  page?: DirectoryPage
  selectedEntryIDs: Set<string>
  canMutate: boolean
  canDelete: boolean
  onOpen: (entry: FileEntry) => void
  onSelectionChange: (entry: FileEntry, selected: boolean) => void
  onAction: (verb: MutationVerb, entry: FileEntry) => void
}) {
  if (error) {
    return (
      <Empty>
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <FileQuestion />
          </EmptyMedia>
          <EmptyTitle>Workspace request failed</EmptyTitle>
          <EmptyDescription>{error}</EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }
  if (loading) {
    return (
      <div
        className="flex flex-col gap-2"
        aria-label="Loading directory"
        role="status"
      >
        {Array.from({ length: 6 }, (_, index) => (
          <Skeleton key={index} className="h-10 w-full" />
        ))}
      </div>
    )
  }
  if (!root) {
    return (
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
    )
  }
  if (!page || page.entries.length === 0) {
    return (
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
    )
  }
  return (
    <DirectoryTable
      entries={page.entries}
      selectedEntryIDs={selectedEntryIDs}
      canMutate={canMutate}
      canDelete={canDelete}
      onOpen={onOpen}
      onSelectionChange={onSelectionChange}
      onAction={onAction}
    />
  )
}

function DirectoryPagination({
  page,
  cursorIndex,
  loading,
  onPrevious,
  onNext,
}: {
  page?: DirectoryPage
  cursorIndex: number
  loading: boolean
  onPrevious: () => void
  onNext: () => void
}) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-3">
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
          onClick={onPrevious}
        >
          <ChevronLeft data-icon="inline-start" /> Previous
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={!page?.has_more || !page.next_cursor || loading}
          onClick={onNext}
        >
          Next <ChevronRight data-icon="inline-end" />
        </Button>
      </div>
    </div>
  )
}
