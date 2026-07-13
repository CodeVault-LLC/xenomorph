import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { type MetadataResult } from "@/lib/files"

import { formatBytes, formatModifiedAt } from "./shared"

export function DetailsInspector({
  metadata,
  selectedPath,
  loading,
  error,
}: {
  metadata?: MetadataResult
  selectedPath: string
  loading: boolean
  error: string
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Details</CardTitle>
        <CardDescription>
          {selectedPath ||
            "Select an entry to inspect client-authored metadata."}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {error ? (
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
        ) : loading && selectedPath ? (
          <Skeleton className="h-28 w-full" />
        ) : metadata ? (
          <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-2 text-sm">
            <dt className="text-muted-foreground">Kind</dt>
            <dd>{metadata.kind}</dd>
            <dt className="text-muted-foreground">Size</dt>
            <dd>{formatBytes(metadata.size)}</dd>
            <dt className="text-muted-foreground">Modified</dt>
            <dd>{formatModifiedAt(metadata.modified_at)}</dd>
            <dt className="text-muted-foreground">Mode</dt>
            <dd className="font-mono">{metadata.mode.toString(8)}</dd>
          </dl>
        ) : (
          <p className="text-sm text-muted-foreground">No entry selected.</p>
        )}
      </CardContent>
    </Card>
  )
}
