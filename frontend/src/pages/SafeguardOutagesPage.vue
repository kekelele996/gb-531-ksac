<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { CalendarClock, FileClock, PauseCircle, RefreshCw, RotateCcw, TimerOff, Wrench } from 'lucide-vue-next'
import AppShell from '../components/common/AppShell.vue'
import PageHeader from '../components/common/PageHeader.vue'
import { useAuth } from '../hooks/useAuth'
import { useSafeguardOutageStore } from '../stores/safeguard-outage'
import { useSafeguardStore } from '../stores/safeguard'
import { useDeviationScenarioStore } from '../stores/deviation-scenario'
import { errorMessage } from '../api/client'
import type { SafeguardOutage, SafeguardOutageInput, SafeguardOutageState } from '../types/safeguard-outage'
import { safeguardOutageStateLabels as stateLabels } from '../types/enums/safeguard-outage-state'

const store = useSafeguardOutageStore()
const safeguards = useSafeguardStore()
const scenarios = useDeviationScenarioStore()
const { canReview } = useAuth()

const filterStatus = ref<SafeguardOutageState | ''>('')
const registerDialog = ref(false)
const registerTargetId = ref<number>()
const saving = ref(false)
const detail = ref<SafeguardOutage>()
const detailOpen = ref(false)
const form = reactive<SafeguardOutageInput>({ reason: '', starts_at: '', ends_at: '' })

const visible = computed(() => filterStatus.value ? store.items.filter((x) => x.status === filterStatus.value) : store.items)
const counts = computed(() => ({
  active: store.items.filter((x) => x.status === 'active').length,
  pending: store.items.filter((x) => x.status === 'pending').length,
  ended: store.items.filter((x) => x.status === 'ended').length,
}))
const safeguardName = (id: number) => safeguards.items.find((x) => x.id === id)?.name ?? `保护层 #${id}`
const scenarioOf = (safeguardId: number) => safeguards.items.find((x) => x.id === safeguardId)?.target_scenario_id
const scenarioLabel = (id?: number) => {
  if (!id) return '—'
  const item = scenarios.items.find((x) => x.id === id)
  return item ? `#${item.id} ${item.guideword.toUpperCase()} ${item.parameter}` : `#${id}`
}
const formatTime = (value: string) => new Date(value).toLocaleString('zh-CN', { hour12: false })

async function refresh() {
  try {
    await Promise.all([store.load(), safeguards.load(), scenarios.load()])
  } catch (error) {
    ElMessage.error(errorMessage(error))
  }
}
function openRegister(safeguardId?: number) {
  registerTargetId.value = safeguardId ?? safeguards.items[0]?.id
  const start = new Date()
  start.setMinutes(0, 0, 0)
  start.setHours(start.getHours() + 1)
  const end = new Date(start.getTime() + 8 * 3600_000)
  Object.assign(form, {
    reason: '',
    starts_at: start.toISOString(),
    ends_at: end.toISOString(),
  })
  registerDialog.value = true
}
async function submitRegister() {
  if (!registerTargetId.value) return ElMessage.warning('请先选择保护措施')
  if (!form.reason.trim()) return ElMessage.warning('请填写停用原因')
  if (!form.starts_at || !form.ends_at) return ElMessage.warning('请填写起止时间')
  saving.value = true
  try {
    await store.register(registerTargetId.value, { ...form, reason: form.reason.trim() })
    ElMessage.success('停用窗口已登记')
    registerDialog.value = false
  } catch (error) {
    ElMessage.error(errorMessage(error))
  } finally {
    saving.value = false
  }
}
async function revoke(item: SafeguardOutage) {
  try {
    const { value } = await ElMessageBox.prompt('撤销后该窗口立即结束，措施重新纳入评估；登记记录保留可查。', '撤销停用登记', {
      confirmButtonText: '确认撤销',
      cancelButtonText: '取消',
      inputPlaceholder: '请填写撤销原因（检修提前完成 / 计划取消等）',
      inputValidator: (value: string) => (value && value.trim().length >= 3 ? true : '撤销原因至少 3 个字符'),
    })
    await store.revoke(item.id, value.trim())
    ElMessage.success('停用登记已撤销')
  } catch (error) {
    if (error !== 'cancel') ElMessage.error(errorMessage(error))
  }
}
onMounted(refresh)
</script>

<template>
  <AppShell>
    <PageHeader eyebrow="PLANNED MAINTENANCE OUTAGE LEDGER" title="保护层停用台账" description="登记计划检修的停用原因与起止时间；停用窗口内的措施不参与独立性去重和覆盖计分，窗口结束后自动重新纳入。">
      <el-button :loading="store.loading" @click="refresh"><RefreshCw :size="16" />刷新</el-button>
      <el-button v-if="canReview" type="primary" @click="openRegister()"><CalendarClock :size="16" />登记停用</el-button>
    </PageHeader>

    <section class="outage-metrics">
      <div :class="{ on: filterStatus === 'active' }" @click="filterStatus = filterStatus === 'active' ? '' : 'active'">
        <TimerOff :size="17" /><span>停用中</span><strong>{{ counts.active }}</strong>
      </div>
      <div :class="{ on: filterStatus === 'pending' }" @click="filterStatus = filterStatus === 'pending' ? '' : 'pending'">
        <PauseCircle :size="17" /><span>待停用</span><strong>{{ counts.pending }}</strong>
      </div>
      <div :class="{ on: filterStatus === 'ended' }" @click="filterStatus = filterStatus === 'ended' ? '' : 'ended'">
        <FileClock :size="17" /><span>已结束</span><strong>{{ counts.ended }}</strong>
      </div>
      <div class="metric-note"><Wrench :size="15" /><span>后提交的重叠时段不能生效；端点相接（前窗结束=后窗开始）允许登记。</span></div>
    </section>

    <section class="data-section">
      <el-table v-loading="store.loading" :data="visible" row-key="id" empty-text="暂无停用登记">
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <span class="state-label" :class="`outage-${row.status}`">{{ stateLabels[row.status as SafeguardOutageState] }}</span>
          </template>
        </el-table-column>
        <el-table-column label="保护措施 / 覆盖目标" min-width="240">
          <template #default="{ row }">
            <div class="primary-cell">
              <strong>{{ safeguardName(row.safeguard_id) }}</strong>
              <span>{{ scenarioLabel(scenarioOf(row.safeguard_id)) }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="停用原因" min-width="220">
          <template #default="{ row }"><span class="outage-reason">{{ row.reason }}</span></template>
        </el-table-column>
        <el-table-column label="起始时间" min-width="165">
          <template #default="{ row }"><span class="date-cell">{{ formatTime(row.starts_at) }}</span></template>
        </el-table-column>
        <el-table-column label="结束时间" min-width="165">
          <template #default="{ row }"><span class="date-cell" :class="{ 'outage-active-text': row.status === 'active' }">{{ formatTime(row.ends_at) }}</span></template>
        </el-table-column>
        <el-table-column label="登记 / 撤销" min-width="200">
          <template #default="{ row }">
            <div class="primary-cell">
              <span>登记：{{ row.registered_by_name }} · {{ formatTime(row.created_at) }}</span>
              <span v-if="row.revoked_at">撤销：{{ row.revoked_by_name || `用户 #${row.revoked_by}` }} · {{ formatTime(row.revoked_at) }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="记录" width="80" fixed="right">
          <template #default="{ row }"><el-button circle text aria-label="查看登记记录" @click="detail = row; detailOpen = true"><FileClock :size="16" /></el-button></template>
        </el-table-column>
        <el-table-column label="操作" width="100" fixed="right">
          <template #default="{ row }">
            <el-tooltip v-if="canReview && row.status !== 'ended'" content="提前撤销，措施立即重新纳入评估">
              <el-button circle text type="warning" aria-label="撤销停用" @click="revoke(row)"><RotateCcw :size="16" /></el-button>
            </el-tooltip>
            <span v-else-if="row.status === 'ended'" class="cell-note">{{ row.revoked_at ? '已撤销' : '自然到期' }}</span>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <el-dialog v-model="registerDialog" title="登记保护层计划停用" width="min(620px, 94vw)">
      <el-form label-position="top" @submit.prevent="submitRegister">
        <el-form-item label="保护措施" required>
          <el-select v-model="registerTargetId" placeholder="选择保护措施">
            <el-option v-for="item in safeguards.items" :key="item.id" :label="`#${item.id} ${item.name}`" :value="item.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="停用原因（检修内容 / 旁路依据）" required>
          <el-input v-model="form.reason" type="textarea" :rows="3" maxlength="1000" show-word-limit placeholder="例如：年度 SIS 逻辑回路校验，停车窗口内信号旁路" />
        </el-form-item>
        <div class="form-grid two">
          <el-form-item label="起始时间" required>
            <el-date-picker v-model="form.starts_at" type="datetime" value-format="YYYY-MM-DDTHH:mm:ssZ" />
          </el-form-item>
          <el-form-item label="结束时间" required>
            <el-date-picker v-model="form.ends_at" type="datetime" value-format="YYYY-MM-DDTHH:mm:ssZ" />
          </el-form-item>
        </div>
        <p class="read-only-note">与同一措施已有窗口重叠的登记会被服务端拒绝（409）；窗口内该措施不参与覆盖评估，结束后无需人工恢复即自动重新纳入。</p>
      </el-form>
      <template #footer>
        <el-button @click="registerDialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submitRegister">登记停用窗口</el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="detailOpen" :title="detail ? `停用登记 #${detail.id}` : '停用登记'" size="min(440px, 92vw)">
      <template v-if="detail">
        <dl class="evidence-pairs">
          <div><dt>状态</dt><dd><span class="state-label" :class="`outage-${detail.status}`">{{ stateLabels[detail.status] }}</span></dd></div>
          <div><dt>保护措施</dt><dd>{{ safeguardName(detail.safeguard_id) }}</dd></div>
          <div class="wide"><dt>停用原因</dt><dd>{{ detail.reason }}</dd></div>
          <div><dt>起始时间</dt><dd>{{ formatTime(detail.starts_at) }}</dd></div>
          <div><dt>结束时间</dt><dd>{{ formatTime(detail.ends_at) }}</dd></div>
          <div><dt>登记人</dt><dd>{{ detail.registered_by_name }}（#{{ detail.registered_by }}）</dd></div>
          <div><dt>登记时间</dt><dd>{{ formatTime(detail.created_at) }}</dd></div>
          <template v-if="detail.revoked_at">
            <div><dt>撤销人</dt><dd>{{ detail.revoked_by_name || `用户 #${detail.revoked_by}` }}</dd></div>
            <div><dt>撤销时间</dt><dd>{{ formatTime(detail.revoked_at) }}</dd></div>
            <div class="wide"><dt>撤销原因</dt><dd>{{ detail.revoke_reason }}</dd></div>
          </template>
        </dl>
      </template>
    </el-drawer>
  </AppShell>
</template>
