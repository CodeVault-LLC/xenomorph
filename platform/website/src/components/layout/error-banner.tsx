import * as React from "react"
import { Check, Copy, RefreshCw, ServerCrash, Share2 } from "lucide-react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"

type ErrorBannerProps = {
  message: string | null
  onRetry?: () => void | Promise<unknown>
  requestPath?: string
  className?: string
}

export function ErrorBanner({
  message,
  onRetry,
  requestPath = "/api/clients",
  className,
}: ErrorBannerProps) {
  const [reportReady, setReportReady] = React.useState(false)

  if (!message) {
    return null
  }

  const isGatewayUnavailable = /HTTP (502|503|504)/.test(message)
  const report = buildDiagnosticReport(message, requestPath)

  async function shareDiagnosticReport() {
    try {
      if (navigator.share) {
        await navigator.share({
          title: "Xenomorph dashboard diagnostic report",
          text: report,
        })
      } else {
        await navigator.clipboard.writeText(report)
      }
      setReportReady(true)
    } catch {
      setReportReady(false)
    }
  }

  return (
    <Alert variant="destructive" className={cn(className)}>
      <ServerCrash />
      <AlertTitle>
        {isGatewayUnavailable
          ? "Gateway unavailable"
          : "Unable to refresh data"}
      </AlertTitle>
      <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
        <span>
          {isGatewayUnavailable
            ? "The gateway is not responding. Your agent was not changed."
            : "The dashboard could not refresh this information. Your agent was not changed."}
        </span>
        <span className="flex items-center gap-2">
          {onRetry ? (
            <Button type="button" variant="outline" size="sm" onClick={onRetry}>
              <RefreshCw data-icon="inline-start" />
              Try again
            </Button>
          ) : null}
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  aria-label="Share diagnostic details"
                  onClick={shareDiagnosticReport}
                />
              }
            >
              {reportReady ? <Check /> : navigator.share ? <Share2 /> : <Copy />}
            </TooltipTrigger>
            <TooltipContent>
              {reportReady
                ? "Diagnostic details ready to send"
                : "Send diagnostic details to support"}
            </TooltipContent>
          </Tooltip>
        </span>
      </AlertDescription>
    </Alert>
  )
}

function buildDiagnosticReport(message: string, requestPath: string) {
  return [
    "Xenomorph dashboard diagnostic report",
    `Time: ${new Date().toISOString()}`,
    `Page: ${window.location.pathname}`,
    `Request: ${requestPath}`,
    `Error: ${message.replace(/\s+/g, " ").trim().slice(0, 500)}`,
  ].join("\n")
}
