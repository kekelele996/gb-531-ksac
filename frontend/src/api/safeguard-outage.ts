import { api, json, query } from './client'
import { normalizePage, type PageData } from '../types/common'
import type { SafeguardOutage, SafeguardOutageInput, SafeguardOutageState } from '../types/safeguard-outage'

export async function listSafeguardOutages(params?: {
  safeguard_id?: number
  scenario_id?: number
  status?: SafeguardOutageState
}): Promise<PageData<SafeguardOutage>> {
  return normalizePage(await api<PageData<SafeguardOutage> | SafeguardOutage[]>(`/safeguard-outages${query(params ?? {})}`))
}
export const getSafeguardOutage = (id: number) => api<SafeguardOutage>(`/safeguard-outages/${id}`)
export const registerSafeguardOutage = (safeguardId: number, input: SafeguardOutageInput) =>
  api<SafeguardOutage>(`/safeguard-outages/safeguards/${safeguardId}`, json('POST', input))
export const revokeSafeguardOutage = (id: number, reason: string) =>
  api<SafeguardOutage>(`/safeguard-outages/${id}/revoke`, json('POST', { reason }))
