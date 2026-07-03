export type ScreenFrameStatus = {
  has_frame: boolean
  agent_id: string
  command_id?: string
  captured_at?: string
  content_type?: string
  image_url?: string
}

export async function fetchScreenFrameStatus(agentID: string) {
  const response = await fetch(`/api/clients/${agentID}/screen/latest`, {
    cache: "no-store",
  })
  if (!response.ok) {
    throw new Error(`Screen status returned HTTP ${response.status}`)
  }

  return (await response.json()) as ScreenFrameStatus
}

export function screenStreamURL(agentID: string) {
  return `/api/clients/${agentID}/screen/stream`
}

export function screenLiveURL(agentID: string) {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:"
  return `${protocol}//${window.location.host}/api/clients/${agentID}/screen/live`
}
