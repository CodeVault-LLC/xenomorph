export type ClientBuildRequest = {
  endpoint: string
  tls_server_name: string
  target_os: "linux" | "darwin" | "windows"
  target_architecture: "amd64" | "arm64"
  client_version: string
}

export type ClientBuildArtifact = {
  contents: Blob
  filename: string
}

type GatewayError = {
  error: string
}

export async function generateClient(
  request: ClientBuildRequest
): Promise<ClientBuildArtifact> {
  const response = await fetch("/api/client-builds", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(request),
  })
  if (!response.ok) {
    throw new Error(await clientBuildError(response))
  }

  return {
    contents: new Blob([await response.arrayBuffer()], {
      type: "application/octet-stream",
    }),
    filename: artifactFilename(request),
  }
}

export function downloadClientArtifact(artifact: ClientBuildArtifact) {
  const url = URL.createObjectURL(artifact.contents)
  const anchor = document.createElement("a")
  anchor.href = url
  anchor.download = artifact.filename
  anchor.hidden = true
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  URL.revokeObjectURL(url)
}

async function clientBuildError(response: Response) {
  const payload: unknown = await response.json().catch(() => undefined)
  if (isGatewayError(payload)) {
    return payload.error
  }

  return `Client build API returned HTTP ${response.status}`
}

function isGatewayError(value: unknown): value is GatewayError {
  return (
    typeof value === "object" &&
    value !== null &&
    "error" in value &&
    typeof value.error === "string" &&
    value.error.length <= 256
  )
}

function artifactFilename(request: ClientBuildRequest) {
  const extension = request.target_os === "windows" ? ".exe" : ""
  return `xenomorph-client-${request.target_os}-${request.target_architecture}${extension}`
}
