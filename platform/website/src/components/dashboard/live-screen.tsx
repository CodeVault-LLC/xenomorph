import { Radio, ScreenShare, Video, WifiOff } from "lucide-react"
import * as React from "react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { type ClientSnapshot, formatRelative } from "@/lib/clients"
import { type ScreenFrameStatus, screenLiveURL } from "@/lib/screen"
import { cn } from "@/lib/utils"

export function LiveScreen({ client }: { client: ClientSnapshot }) {
  const [frame, setFrame] = React.useState<ScreenFrameStatus | null>(null)
  const [error, setError] = React.useState<string | null>(null)
  const [connectedAgentID, setConnectedAgentID] = React.useState<string | null>(
    null
  )
  const canvasRef = React.useRef<HTMLCanvasElement | null>(null)
  const videoRef = React.useRef<HTMLVideoElement | null>(null)
  const sequenceRef = React.useRef(0)

  const drawFrame = React.useCallback(async (blob: Blob) => {
    const canvas = canvasRef.current
    if (!canvas) {
      return
    }

    const currentSequence = sequenceRef.current + 1
    sequenceRef.current = currentSequence
    const bitmap = await createImageBitmap(blob)
    if (sequenceRef.current !== currentSequence) {
      bitmap.close()
      return
    }

    canvas.width = bitmap.width
    canvas.height = bitmap.height
    canvas.getContext("2d")?.drawImage(bitmap, 0, 0)
    bitmap.close()
  }, [])

  React.useEffect(() => {
    const canvas = canvasRef.current
    const video = videoRef.current
    if (!canvas || !video) {
      return
    }

    const stream = canvas.captureStream(60)
    video.srcObject = stream
    void video.play()
    return () => {
      stream.getTracks().forEach((track) => track.stop())
      video.srcObject = null
    }
  }, [])

  React.useEffect(() => {
    if (!client.is_online) {
      return
    }

    const socket = new WebSocket(screenLiveURL(client.agent_id))
    socket.binaryType = "blob"

    socket.onopen = () => {
      setConnectedAgentID(client.agent_id)
      setError(null)
    }

    socket.onmessage = (event) => {
      if (!(event.data instanceof Blob)) {
        return
      }
      const capturedAt = new Date().toISOString()
      setFrame({
        has_frame: true,
        agent_id: client.agent_id,
        captured_at: capturedAt,
        content_type: event.data.type || "image/png",
      })
      setConnectedAgentID(client.agent_id)
      setError(null)
      void drawFrame(event.data)
    }

    socket.onerror = () => {
      setConnectedAgentID(null)
      setError("Screen stream disconnected")
    }

    socket.onclose = () => {
      setConnectedAgentID(null)
    }

    return () => {
      socket.close()
    }
  }, [client.agent_id, client.is_online, drawFrame])

  const hasFrame = Boolean(
    frame?.has_frame && frame.agent_id === client.agent_id
  )
  const connected = connectedAgentID === client.agent_id

  return (
    <Card className="overflow-hidden">
      <CardHeader className="flex-row items-center justify-between gap-4">
        <div>
          <CardTitle>Live Screen</CardTitle>
          <CardDescription>
            {client.is_online
              ? "Streams authenticated live screen media through the gateway."
              : `Offline since ${formatRelative(client.last_online)}.`}
          </CardDescription>
        </div>
        <div className="flex items-center gap-2">
          <Badge variant={client.is_online && connected ? "online" : "offline"}>
            {client.is_online && connected ? "LIVE" : "OFFLINE"}
          </Badge>
        </div>
      </CardHeader>
      <CardContent>
        <div
          className={cn(
            "relative flex aspect-video min-h-[320px] items-center justify-center overflow-hidden rounded-md border bg-zinc-950",
            !hasFrame && "border-dashed bg-muted/50 p-6 text-center"
          )}
        >
          <canvas ref={canvasRef} className="hidden" />
          <video
            ref={videoRef}
            aria-label="Live agent screen video"
            autoPlay
            controls
            muted
            playsInline
            className={cn(
              "h-full w-full bg-zinc-950 object-contain",
              !hasFrame && "hidden"
            )}
          />
          {!hasFrame ? (
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  {client.is_online ? <ScreenShare /> : <WifiOff />}
                </EmptyMedia>
                <EmptyTitle>
                  {client.is_online
                    ? "Opening live stream"
                    : "No active session"}
                </EmptyTitle>
                <EmptyDescription>
                  {client.is_online
                    ? "The video player starts as soon as the next authenticated frame reaches the gateway."
                    : "Live video is disabled until the agent returns online."}
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : null}
          {hasFrame ? (
            <div className="pointer-events-none absolute top-3 left-3 inline-flex items-center gap-2 rounded-md border border-white/15 bg-black/60 px-2.5 py-1 text-xs font-medium text-white">
              <Video />
              Live
            </div>
          ) : null}
        </div>

        <div className="mt-3 flex flex-wrap items-center justify-between gap-3 text-sm text-muted-foreground">
          <span className="inline-flex items-center gap-2">
            <Radio
              className={cn(
                "size-4",
                client.is_online && connected && "text-primary"
              )}
            />
            {frame?.has_frame && frame.captured_at
              ? `Captured ${formatRelative(frame.captured_at)}`
              : client.is_online
                ? "Waiting for first live frame"
                : "No frame captured yet"}
          </span>
          {client.is_online && error ? (
            <Alert variant="destructive" className="w-auto">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
        </div>
      </CardContent>
    </Card>
  )
}
