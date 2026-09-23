export const safeguardOutageStates = ['pending', 'active', 'ended'] as const
export type SafeguardOutageState = (typeof safeguardOutageStates)[number]

export const safeguardOutageStateLabels: Record<SafeguardOutageState, string> = {
  pending: '待停用',
  active: '停用中',
  ended: '已结束',
}
