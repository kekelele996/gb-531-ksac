import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as outageApi from '../api/safeguard-outage'
import type { SafeguardOutage, SafeguardOutageInput, SafeguardOutageStatus } from '../types/safeguard-outage'

export const useSafeguardOutageStore = defineStore('safeguard-outages', () => {
  const items = ref<SafeguardOutage[]>([])
  const loading = ref(false)
  async function load(filter?: { safeguard_id?: number; scenario_id?: number; status?: SafeguardOutageStatus }) {
    loading.value = true
    try {
      items.value = (await outageApi.listSafeguardOutages(filter)).items
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
  return { items, loading, load, register, revoke }
})
