import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Settings2 } from "lucide-react"
import { type MetadataResult, type RootCapabilities } from "@/lib/files"

import { formatBytes, formatModifiedAt } from "./shared"

export function DetailsInspector({
  metadata,
  selectedPath,
  loading,
  error,
  capabilities,
  onEdit,
}: {
  metadata?: MetadataResult
  selectedPath: string
  loading: boolean
  error: string
  capabilities?: RootCapabilities
  onEdit: () => void
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
          <>
            <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-2 text-sm">
              <dt className="text-muted-foreground">Kind</dt>
              <dd>{metadata.kind}</dd>
              <dt className="text-muted-foreground">Size</dt>
              <dd>{formatBytes(metadata.size)}</dd>
              <dt className="text-muted-foreground">Modified</dt>
              <dd>{formatModifiedAt(metadata.modified_at)}</dd>
              <dt className="text-muted-foreground">Mode</dt>
              <dd className="font-mono">{metadata.mode.toString(8)}</dd>
              {Object.entries(metadata.optional_fields)
                .slice(0, 16)
                .map(([field, value]) => (
                  <OptionalMetadata key={field} field={field} value={value} />
                ))}
            </dl>
            <Button
              variant="outline"
              size="sm"
              disabled={capabilities?.metadata_write !== "available"}
              title={
                capabilities?.metadata_write === "available"
                  ? undefined
                  : `Metadata writes are ${capabilities?.metadata_write ?? "unavailable"} on this root`
              }
              onClick={onEdit}
            >
              <Settings2 data-icon="inline-start" /> Edit metadata
            </Button>
            {capabilities?.metadata_write !== "available" ? (
              <p className="text-xs text-muted-foreground">
                Metadata editing is{" "}
                {capabilities?.metadata_write ?? "unavailable"}. Unsupported
                fields remain unchanged.
              </p>
            ) : null}
          </>
        ) : (
          <p className="text-sm text-muted-foreground">No entry selected.</p>
        )}
      </CardContent>
    </Card>
  )
}

function OptionalMetadata({
  field,
  value,
}: {
  field: string
  value: MetadataResult["optional_fields"][string]
}) {
  const label = field.replaceAll("_", " ")
  return (
    <>
      <dt className="text-muted-foreground capitalize">{label}</dt>
      <dd>
        {value.state === "available" && value.value ? (
          <span className="font-mono text-xs break-all">{value.value}</span>
        ) : (
          <Badge variant="outline">{value.state}</Badge>
        )}
      </dd>
    </>
  )
}
