import {
  ChevronLeft,
  ChevronRight,
  FilePlus2,
  FileQuestion,
  FolderOpen,
  FolderPlus,
  HardDrive,
  Search,
  Trash2,
  Upload,
  X,
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
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group"
import type {
  DirectoryPage,
  DirectorySearchResult,
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
  searchQuery: string
  searchResult?: DirectorySearchResult
  searchLoading: boolean
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
  onSearchQueryChange: (query: string) => void
  onSearch: () => void
  onClearSearch: () => void
  onRootChange: (root: FileRoot) => void
  onNavigate: (path: string) => void
  onOpen: (entry: FileEntry) => void
  onOpenSearchResult: (result: DirectorySearchResult["entries"][number]) => void
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
  searchQuery,
  searchResult,
  searchLoading,
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
  onSearchQueryChange,
  onSearch,
  onClearSearch,
  onRootChange,
  onNavigate,
  onOpen,
  onOpenSearchResult,
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
        <form
          className="flex flex-wrap items-center gap-2"
          onSubmit={(event) => {
            event.preventDefault()
            onSearch()
          }}
        >
          <InputGroup className="min-w-64 flex-1">
            <InputGroupInput
              aria-label="Search this folder and subfolders"
              placeholder="Search this folder and subfolders…"
              value={searchQuery}
              minLength={2}
              maxLength={256}
              onChange={(event) => onSearchQueryChange(event.target.value)}
            />
            <InputGroupAddon>
              <Search />
            </InputGroupAddon>
            {searchQuery || searchResult ? (
              <InputGroupAddon align="inline-end">
                <InputGroupButton
                  type="button"
                  size="icon-xs"
                  aria-label="Clear search"
                  onClick={onClearSearch}
                >
                  <X />
                </InputGroupButton>
              </InputGroupAddon>
            ) : null}
          </InputGroup>
          <Button
            type="submit"
            variant="secondary"
            disabled={searchQuery.trim().length < 2 || searchLoading}
          >
            <Search data-icon="inline-start" />
            {searchLoading ? "Searching…" : "Search"}
          </Button>
        </form>
        <DirectoryContent
          error={error}
          loading={loading}
          root={root}
          page={page}
          searchResult={searchResult}
          searchLoading={searchLoading}
          selectedEntryIDs={selectedEntryIDs}
          canMutate={canMutate}
          canDelete={canDelete}
          onOpen={onOpen}
          onOpenSearchResult={onOpenSearchResult}
          onSelectionChange={onSelectionChange}
          onAction={onAction}
        />
        {!searchResult ? (
          <DirectoryPagination
            page={page}
            cursorIndex={cursorIndex}
            loading={loading}
            onPrevious={onPreviousPage}
            onNext={onNextPage}
          />
        ) : null}
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
  searchResult,
  searchLoading,
  selectedEntryIDs,
  canMutate,
  canDelete,
  onOpen,
  onOpenSearchResult,
  onSelectionChange,
  onAction,
}: {
  error: string
  loading: boolean
  root?: FileRoot
  page?: DirectoryPage
  searchResult?: DirectorySearchResult
  searchLoading: boolean
  selectedEntryIDs: Set<string>
  canMutate: boolean
  canDelete: boolean
  onOpen: (entry: FileEntry) => void
  onOpenSearchResult: (result: DirectorySearchResult["entries"][number]) => void
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
  if (loading || searchLoading) {
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
  if (searchResult) {
    if (searchResult.entries.length === 0) {
      return (
        <Empty>
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <Search />
            </EmptyMedia>
            <EmptyTitle>No matching files</EmptyTitle>
            <EmptyDescription>
              Search inspected {searchResult.scanned_entries.toLocaleString()}{" "}
              entries within the bounded subtree.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )
    }
    return (
      <div className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
          <span>
            {searchResult.entries.length.toLocaleString()} matches across{" "}
            {searchResult.scanned_entries.toLocaleString()} inspected entries
          </span>
          {searchResult.truncated ? (
            <Badge variant="outline">Bound reached · refine search</Badge>
          ) : null}
        </div>
        <DirectoryTable
          entries={searchResult.entries.map((result) => result.entry)}
          selectedEntryIDs={new Set()}
          canMutate={false}
          canDelete={false}
          onOpen={(entry) => {
            const result = searchResult.entries.find(
              (candidate) => candidate.entry.entry_id === entry.entry_id
            )
            if (result) onOpenSearchResult(result)
          }}
          onSelectionChange={() => undefined}
          onAction={() => undefined}
        />
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
