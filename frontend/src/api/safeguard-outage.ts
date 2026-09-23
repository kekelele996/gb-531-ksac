import { api, json, query } from './client'
import { normalizePage, type PageData } from '../types/common'
import type { SafeguardOutage, SafeguardOutageInput, SafeguardOutageStatus } from '../types/safeguard-outage'

export async function listSafeguardOutages(filter?: {
  safeguard_id?: number
  scenario_id?: number
  status?: SafeguardOutageStatus
}): Promise<PageData<SafeguardOutage>> {
  return normalizePage(
    await api<PageData<SafeguardOutage>>(
      `/safeguard-outages${query({
        safeguard_id: filter?.safeguard_id,
        scenario_id: filter?.scenario_id,
        status: filter?.status,
      })}`,
    ),
  )
}
export const registerSafeguardOutage = (safeguardId: number, input: SafeguardOutageInput) =>
  api<SafeguardOutage>(`/safeguards/${safeguardId}/outages`, json('POST', input))
export const revokeSafeguardOutage = (id: number, reason: string) =>
  api<SafeguardOutage>(`/safeguard-outages/${id}/revoke`, json('POST', { reason }))
