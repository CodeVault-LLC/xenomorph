import { ArrowLeft, Download } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { decodePreviewText, type PreviewResult } from "@/lib/files"

type FileViewerProps = {
  preview?: PreviewResult
  selectedPath: string
  loading: boolean
  error: string
  canDownload: boolean
  downloadPending: boolean
  onBack: () => void
  onDownload: () => void
}

export function FileViewer({
  preview,
  selectedPath,
  loading,
  error,
  canDownload,
  downloadPending,
  onBack,
  onDownload,
}: FileViewerProps) {
  const decodedText = preview ? decodePreview(preview) : undefined

  return (
    <Card className="min-w-0">
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="flex min-w-0 flex-col gap-1">
            <CardTitle className="truncate">{selectedPath}</CardTitle>
            <CardDescription>
              Client-authored file content preview.
            </CardDescription>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={onBack}>
              <ArrowLeft data-icon="inline-start" /> Back to files
            </Button>
            <Button
              size="sm"
              disabled={!canDownload || downloadPending}
              onClick={onDownload}
            >
              <Download data-icon="inline-start" />
              {downloadPending ? "Starting…" : "Download"}
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent className="flex flex-col gap-5 pt-4">
        {error ? (
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
        ) : loading ? (
          <div className="flex flex-col gap-3" aria-label="Loading file">
            <Skeleton className="h-20 w-full" />
            <Skeleton className="h-96 w-full" />
          </div>
        ) : preview ? (
          <section
            className="flex min-w-0 flex-col gap-2"
            aria-label="File content"
          >
            {decodedText !== undefined ? (
              <pre className="max-h-130 overflow-auto rounded-lg bg-muted p-4 text-xs whitespace-pre-wrap">
                {decodedText}
              </pre>
            ) : preview.classification === "text" ? (
              <p className="text-sm text-destructive" role="alert">
                File data could not be decoded.
              </p>
            ) : (
              <p className="text-sm text-muted-foreground">
                Binary content is not rendered.
              </p>
            )}
            {preview.truncated ? (
              <p className="text-xs text-muted-foreground">
                The file exceeds the safe preview limit. Download it to view the
                complete content.
              </p>
            ) : null}
          </section>
        ) : (
          <p className="text-sm text-muted-foreground">
            A preview is unavailable for this entry.
          </p>
        )}
      </CardContent>
    </Card>
  )
}

function decodePreview(preview: PreviewResult) {
  if (preview.classification !== "text") return undefined
  try {
    return decodePreviewText(preview)
  } catch {
    return undefined
  }
}
