<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { apiClient } from '../../api/client'
interface Order { id: string; user_id: number; amount_cents: number; credit: string; status: string; reversed_at: string | null; created_at: string; gateway_status: string; cache_synced: boolean; open_issues: number }
interface Issue { id: number; order_id: string; kind: string; detail: string; occurrences: number; updated_at: string; resolved_at: string | null; resolution: string; notified_at: string | null; notify_attempts: number }
const orders = ref<Order[]>([]), issues = ref<Issue[]>([]), search = ref(''), section = ref('issues'), busy = ref(false), error = ref(''), notice = ref('')
const action = ref<{ type: 'recheck' | 'refund' | 'resolve'; id: string; amount?: string } | null>(null)
const reason = ref(''), reference = ref(''), confirmed = ref(false), includeResolved = ref(false)
const visibleOrders = computed(() => orders.value.filter(o => `${o.id} ${o.user_id}`.includes(search.value.trim())))
const visibleIssues = computed(() => issues.value.filter(i => (includeResolved.value || !i.resolved_at) && `${i.order_id} ${i.detail}`.includes(search.value.trim())))
const openCount = computed(() => issues.value.filter(i => !i.resolved_at).length)
const titles = { recheck: '重新查单与补单', refund: '登记已完成的全额退款', resolve: '标记异常已处理' }
const statuses: Record<string, string> = { paid: '已支付', pending: '待支付', expired: '平台超时', not_found: '平台尚无订单' }
function fail(e: any) { error.value = e?.response?.data?.message || e?.message || '请求失败，请稍后重试' }
async function load() {
  try { const [a, b] = await Promise.all([apiClient.get('/recharge/admin/orders'), apiClient.get('/recharge/admin/issues')]); orders.value = a.data; issues.value = b.data }
  catch (e) { fail(e) }
}
function open(type: 'recheck' | 'refund' | 'resolve', id: string, amount?: string) { action.value = { type, id, amount }; reason.value = ''; reference.value = ''; confirmed.value = false; error.value = '' }
async function submit() {
  if (!action.value || busy.value) return
  busy.value = true; error.value = ''; notice.value = ''
  const a = action.value
  try {
    const path = a.type === 'resolve' ? `/recharge/admin/issues/${a.id}/resolve` : `/recharge/admin/orders/${encodeURIComponent(a.id)}/${a.type}`
    const { data } = await apiClient.post(path, { reason: reason.value.trim(), reference: reference.value.trim(), confirmed: confirmed.value })
    notice.value = data.message || (a.type === 'refund' ? '退款登记与余额冲正已完成' : a.type === 'resolve' ? '异常已标记处理，操作已留痕' : '已完成平台查单，确认付款的订单已自动入账')
    action.value = null; await load()
  } catch (e) { fail(e); await load() }
  finally { busy.value = false }
}
onMounted(load)
</script>
<template>
  <section class="admin-panel">
    <div class="admin-heading"><div><h2>充值管理</h2><p>最近 200 条订单与异常 · 管理员操作全程留痕</p></div><button :disabled="busy" @click="load">刷新数据 ↻</button></div>
    <div v-if="error" class="message error" role="alert">{{ error }}</div><div v-if="notice" class="message" role="status">{{ notice }}</div>
    <div class="admin-tools"><div><button :class="{ selected: section === 'issues' }" @click="section = 'issues'">异常订单 {{ openCount }}</button><button :class="{ selected: section === 'orders' }" @click="section = 'orders'">全部订单</button></div><input v-model="search" placeholder="搜索当前列表中的订单号或用户 ID"><label v-if="section === 'issues'"><input v-model="includeResolved" type="checkbox">显示已处理</label></div>
    <div class="admin-table"><table v-if="section === 'issues'"><thead><tr><th>订单 / 最近出现</th><th>异常详情</th><th>告警</th><th>处理</th></tr></thead><tbody><tr v-for="i in visibleIssues" :key="i.id"><td><code>{{ i.order_id }}</code><small>{{ new Date(i.updated_at).toLocaleString('zh-CN') }} · {{ i.occurrences }} 次</small></td><td>{{ i.detail }}<small v-if="i.resolved_at">已处理：{{ i.resolution }}</small></td><td>{{ i.notified_at ? '已推送' : i.notify_attempts ? '等待重试' : '站内可见' }}</td><td><div v-if="!i.resolved_at" class="actions"><button :disabled="busy" @click="open('recheck', i.order_id)">查单补单</button><button :disabled="busy" @click="open('resolve', String(i.id))">标记处理</button></div><span v-else>已处理</span></td></tr></tbody></table>
      <table v-else><thead><tr><th>订单 / 用户</th><th>金额 / 额度</th><th>本地 / 平台状态</th><th>操作</th></tr></thead><tbody><tr v-for="o in visibleOrders" :key="o.id"><td><code>{{ o.id }}</code><small>用户 {{ o.user_id }} · {{ new Date(o.created_at).toLocaleString('zh-CN') }}</small></td><td>¥{{ (o.amount_cents / 100).toFixed(2) }} / ${{ Number(o.credit) }}</td><td>{{ o.reversed_at ? '退款已冲正' : statuses[o.status] || o.status }} / {{ statuses[o.gateway_status] || '尚未查单' }}<small v-if="o.status === 'paid' && !o.cache_synced">缓存刷新待重试</small><small v-if="o.open_issues">{{ o.open_issues }} 项待处理异常</small></td><td><div class="actions"><button v-if="!o.reversed_at" :disabled="busy" @click="open('recheck', o.id)">查单补单</button><button v-if="o.status === 'paid' && !o.reversed_at" :disabled="busy" @click="open('refund', o.id, `$${Number(o.credit)}`)">退款登记</button></div></td></tr></tbody></table>
      <p v-if="section === 'issues' ? !visibleIssues.length : !visibleOrders.length" class="empty">当前列表没有匹配的记录。</p>
    </div>
    <div v-if="action" class="modal-backdrop"><form class="action-modal" role="dialog" aria-modal="true" :aria-label="titles[action.type]" @submit.prevent="submit"><h3>{{ titles[action.type] }}</h3><code>{{ action.id }}</code><p v-if="action.type === 'recheck'">将向支付平台重新核对付款信息。只有金额、支付方式与流水核对一致且已付款，才会补入账。</p><p v-else-if="action.type === 'refund'" class="refund-note">此操作会从用户余额扣回 {{ action.amount }}，不会向支付平台发起退款。请先在商户后台完成全额退款；可用余额不足时将停止冲正并记录异常。</p><p v-else>仅关闭当前异常记录，不会改变付款状态或增加余额。异常再次出现时会重新打开。</p><label>操作原因<textarea v-model="reason" required minlength="3" maxlength="300" placeholder="填写核对结果、处理依据或原因"></textarea></label><template v-if="action.type === 'refund'"><label>支付平台退款凭证 / 流水号<input v-model="reference" required minlength="3" maxlength="128"></label><label class="confirm"><input v-model="confirmed" type="checkbox" required>我已在支付平台完成该订单的全额退款，并核对了凭证</label></template><div v-if="error" class="message error" role="alert">{{ error }}</div><div class="modal-actions"><button type="button" :disabled="busy" @click="action = null">取消</button><button class="primary" :disabled="busy || reason.trim().length < 3 || (action.type === 'refund' && (!confirmed || reference.trim().length < 3))">{{ busy ? '处理中…' : action.type === 'refund' ? '确认登记并扣回余额' : '确认执行' }}</button></div></form></div>
  </section>
</template>
<style scoped>
.admin-heading{display:flex;align-items:center;justify-content:space-between;gap:20px}.admin-heading h2{margin:0;font-size:22px}.admin-heading p{font-size:13px;color:#888c98}button{border:1px solid #dfe3ec;background:#fff;color:#353b49;border-radius:8px;padding:9px 13px;font-size:12px;white-space:nowrap}button:disabled{opacity:.45;cursor:not-allowed}.admin-tools{display:flex;gap:16px;align-items:center;flex-wrap:wrap;margin:22px 0}.admin-tools>div{display:flex;gap:8px}.selected{background:#eef5ff;border-color:#bbd2fc;color:#2864e8}input:not([type=checkbox]),textarea{border:1px solid #dfe3ec;background:white;color:#252936;border-radius:8px;padding:10px 12px;font:inherit}.admin-tools>input{font-size:13px;min-width:280px}.admin-tools label{font-size:12px;color:#707786}.admin-table{background:white;border:1px solid #e4e7ed;border-radius:12px;overflow-x:auto}table{width:100%;border-collapse:collapse;font-size:12px;text-align:left}td,th{padding:16px;border-bottom:1px solid #eceef4}th{background:#f5f6fa;color:#858b99;font-weight:500}td{max-width:400px;line-height:1.7}td small{display:block;color:#8b90a0;margin-top:6px}code{font-size:11px;overflow-wrap:anywhere}.actions{display:flex;gap:8px}.empty{text-align:center;padding:35px;color:#939aaa}.message{padding:13px 16px;margin:15px 0;border-radius:8px;color:#2864ad;background:#eef6ff;font-size:13px}.error{color:#ad3333;background:#fff0f0}.modal-backdrop{position:fixed;inset:0;background:#131a3280;display:flex;align-items:center;justify-content:center;z-index:100;padding:20px}.action-modal{width:520px;max-width:100%;max-height:90vh;overflow:auto;background:#fff;border-radius:16px;padding:28px;box-shadow:0 10px 80px #10162c40}.action-modal h3{margin-top:0}.action-modal p{font-size:13px;line-height:1.8;color:#767e8d}.action-modal label{display:flex;flex-direction:column;gap:9px;font-size:13px;margin-top:18px}.action-modal textarea{min-height:90px;resize:vertical}.action-modal .confirm{flex-direction:row;align-items:flex-start;line-height:1.7}.action-modal .refund-note{color:#a65d24}.modal-actions{display:flex;justify-content:flex-end;gap:12px;margin-top:25px}.primary{background:#242731;color:white;border-color:#242731}@media(max-width:600px){.admin-heading{align-items:flex-start}.admin-tools>input{min-width:0;width:100%}.action-modal{padding:20px}.actions{flex-direction:column}}
</style>
