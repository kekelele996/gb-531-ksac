import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import * as outageApi from '../api/safeguard-outage'
import type { SafeguardOutage, SafeguardOutageInput, SafeguardOutageState } from '../types/safeguard-outage'

export const useSafeguardOutageStore = defineStore('safeguard-outages', () => {
  const items = ref<SafeguardOutage[]>([])
  const loading = ref(false)

  async function load(params?: { safeguard_id?: number; scenario_id?: number; status?: SafeguardOutageState }) {
    loading.value = true
    try {
      items.value = (await outageApi.listSafeguardOutages(params)).items
    } finally {
      loading.value = false
    }
  }
  async function register(safeguardId: number, input: SafeguardOutageInput) {
    const item = await outageApi.registerSafeguardOutage(safeguardId, input)
    await load()
    return item
  }
  async function revoke(id: number, reason: string) {
    const item = await outageApi.revokeSafeguardOutage(id, reason)
    await load()
    return item
  }

  const forSafeguard = computed(() => (safeguardId: number) =>
    items.value
      .filter((item) => item.safeguard_id === safeguardId)
      .sort((a, b) => b.starts_at.localeCompare(a.starts_at)))
  // At most one window can be active per safeguard because overlapping windows
  // are rejected at registration.
  const activeForSafeguard = computed(() => (safeguardId: number) =>
    items.value.find((item) => item.safeguard_id === safeguardId && item.status === 'active'))
  const pendingForSafeguard = computed(() => (safeguardId: number) =>
    items.value.filter((item) => item.safeguard_id === safeguardId && item.status === 'pending'))

  return { items, loading, load, register, revoke, forSafeguard, activeForSafeguard, pendingForSafeguard }
})
