import { type FormEvent, useState } from "react"
import { useMutation } from "@tanstack/react-query"
import { DownloadIcon } from "lucide-react"
import { createFileRoute } from "@tanstack/react-router"

import {
  type ClientBuildRequest,
  downloadClientArtifact,
  generateClient,
} from "@/lib/client-builds"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { PageHeader } from "@/components/layout/page-header"
import { PageShell } from "@/components/layout/page-shell"

export const Route = createFileRoute("/generate-client")({
  component: GenerateClientRoute,
})

const targetOperatingSystems = [
  { value: "linux", label: "Linux" },
  { value: "darwin", label: "macOS" },
  { value: "windows", label: "Windows" },
] as const

const targetArchitectures = [
  { value: "amd64", label: "x86-64 (amd64)" },
  { value: "arm64", label: "ARM64" },
] as const

function GenerateClientRoute() {
  const [request, setRequest] = useState<ClientBuildRequest>({
    endpoint: "",
    tls_server_name: "",
    target_os: "linux",
    target_architecture: "amd64",
    client_version: "",
  })
  const [validationError, setValidationError] = useState<string | null>(null)
  const buildMutation = useMutation({
    mutationFn: generateClient,
    onSuccess: downloadClientArtifact,
  })

  function updateTextField(
    field: "endpoint" | "tls_server_name" | "client_version",
    value: string
  ) {
    setRequest((current) => ({ ...current, [field]: value }))
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const error = formError(request)
    setValidationError(error)
    if (error) {
      return
    }

    buildMutation.mutate(request)
  }

  const mutationError =
    buildMutation.error instanceof Error
      ? buildMutation.error.message
      : "Client build failed"

  return (
    <PageShell>
      <PageHeader
        title="Generate client"
        description="Build a client with one compiled gateway endpoint and TLS server name."
      />

      <Card className="max-w-2xl">
        <CardHeader>
          <CardTitle>Client profile</CardTitle>
          <CardDescription>
            These browser-authored values are validated by the gateway before it
            builds the downloadable binary.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} className="flex flex-col gap-5">
            <FieldGroup>
              <Field data-invalid={Boolean(validationError)}>
                <FieldLabel htmlFor="client-endpoint">Endpoint</FieldLabel>
                <Input
                  id="client-endpoint"
                  value={request.endpoint}
                  onChange={(event) =>
                    updateTextField("endpoint", event.target.value)
                  }
                  placeholder="gateway.example.com:8444"
                  autoComplete="off"
                  aria-invalid={Boolean(validationError)}
                  disabled={buildMutation.isPending}
                />
                <FieldDescription>
                  The exact gateway host and port compiled into the client.
                </FieldDescription>
              </Field>

              <Field data-invalid={Boolean(validationError)}>
                <FieldLabel htmlFor="client-tls-name">
                  TLS server name
                </FieldLabel>
                <Input
                  id="client-tls-name"
                  value={request.tls_server_name}
                  onChange={(event) =>
                    updateTextField("tls_server_name", event.target.value)
                  }
                  placeholder="gateway.example.com"
                  autoComplete="off"
                  aria-invalid={Boolean(validationError)}
                  disabled={buildMutation.isPending}
                />
                <FieldDescription>
                  The DNS name used for TLS certificate verification.
                </FieldDescription>
              </Field>

              <Field>
                <FieldLabel>Target operating system</FieldLabel>
                <Select
                  items={targetOperatingSystems}
                  value={request.target_os}
                  disabled={buildMutation.isPending}
                  onValueChange={(value) => {
                    if (
                      value === "linux" ||
                      value === "darwin" ||
                      value === "windows"
                    ) {
                      setRequest((current) => ({
                        ...current,
                        target_os: value,
                      }))
                    }
                  }}
                >
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {targetOperatingSystems.map((target) => (
                        <SelectItem key={target.value} value={target.value}>
                          {target.label}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>

              <Field>
                <FieldLabel>Target architecture</FieldLabel>
                <Select
                  items={targetArchitectures}
                  value={request.target_architecture}
                  disabled={buildMutation.isPending}
                  onValueChange={(value) => {
                    if (value === "amd64" || value === "arm64") {
                      setRequest((current) => ({
                        ...current,
                        target_architecture: value,
                      }))
                    }
                  }}
                >
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {targetArchitectures.map((target) => (
                        <SelectItem key={target.value} value={target.value}>
                          {target.label}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>

              <Field data-invalid={Boolean(validationError)}>
                <FieldLabel htmlFor="client-version">Client version</FieldLabel>
                <Input
                  id="client-version"
                  value={request.client_version}
                  onChange={(event) =>
                    updateTextField("client_version", event.target.value)
                  }
                  placeholder="1.0.0"
                  autoComplete="off"
                  aria-invalid={Boolean(validationError)}
                  disabled={buildMutation.isPending}
                />
                <FieldDescription>
                  A build identity compiled into the generated client.
                </FieldDescription>
              </Field>
            </FieldGroup>

            <FieldError>{validationError}</FieldError>
            {buildMutation.isError ? (
              <Alert variant="destructive">
                <AlertTitle>Client build failed</AlertTitle>
                <AlertDescription>{mutationError}</AlertDescription>
              </Alert>
            ) : null}
            {buildMutation.isSuccess ? (
              <Alert>
                <AlertTitle>Client download started</AlertTitle>
                <AlertDescription>
                  The binary uses the submitted endpoint and TLS name as its
                  compiled profile.
                </AlertDescription>
              </Alert>
            ) : null}

            <Button type="submit" disabled={buildMutation.isPending}>
              {buildMutation.isPending ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <DownloadIcon data-icon="inline-start" />
              )}
              {buildMutation.isPending
                ? "Generating client"
                : "Generate client"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </PageShell>
  )
}

function formError(request: ClientBuildRequest) {
  if (!request.endpoint.trim() || !request.tls_server_name.trim()) {
    return "Endpoint and TLS server name are required."
  }
  if (!request.client_version.trim()) {
    return "Client version is required."
  }

  return null
}
