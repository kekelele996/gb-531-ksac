export type SafeguardOutageStatus = 'scheduled' | 'active' | 'ended' | 'revoked'

export const outageStatusLabels: Record<SafeguardOutageStatus, string> = {
  scheduled: '待停用',
  active: '停用中',
  ended: '已结束',
  revoked: '已撤销',
}

export interface SafeguardOutage {
  id: number
  safeguard_id: number
  safeguard_name: string
  scenario_id: number
  reason: string
  starts_at: string
  ends_at: string
  status: SafeguardOutageStatus
  registered_by: number
  registered_by_name: string
  revoked_by?: number
  revoked_by_name?: string
  revoked_at?: string
  revoke_reason?: string
  created_at: string
}

export interface SafeguardOutageInput {
  reason: string
  starts_at: string
  ends_at: string
}
