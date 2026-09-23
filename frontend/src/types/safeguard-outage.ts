import type { SafeguardOutageState } from './enums/safeguard-outage-state'

export type { SafeguardOutageState }

export interface SafeguardOutage {
  id: number
  safeguard_id: number
  safeguard_name?: string
  reason: string
  starts_at: string
  ends_at: string
  status: SafeguardOutageState
  revoked_at?: string | null
  revoked_by?: number | null
  revoked_by_name?: string
  revoke_reason?: string
  registered_by: number
  registered_by_name: string
  created_at: string
  updated_at: string
}

export interface SafeguardOutageInput {
  reason: string
  starts_at: string
  ends_at: string
}
