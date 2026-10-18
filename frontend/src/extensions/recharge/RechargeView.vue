<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { apiClient } from '../../api/client'
import RechargeAdmin from './RechargeAdmin.vue'
import RechargeDialog from './RechargeDialog.vue'
import type {Package,Order,Log} from './types'
const packages = ref<Package[]>([]), orders = ref<Order[]>([]), logs = ref<Log[]>([])
const tab = ref('products'), busy = ref(false), error = ref(''), notice = ref('')
const balance = ref<string | number>('—'), email = ref(''), authenticated = ref(false), isAdmin = ref(false)
let timer: ReturnType<typeof setInterval> | undefined
const amount = (n: number) => (n / 100).toFixed(2)
const time = (s: string) => new Date(s).toLocaleString('zh-CN')
function logDetail(l: Log): string {
  try {
    const d = JSON.parse(l.detail)
    if (l.event === 'credited') return `到账 $${Number(d.credit)} · 入账后余额 $${Number(d.balance_after)} · 支付流水 ${d.trade_no}`
    if (l.event === 'refund_registered') return `扣回 $${Number(d.debit)} · 冲正后余额 $${Number(d.balance_after)} · 退款凭证 ${d.refund_reference} · 操作人 #${d.actor_id} · ${d.reason}`
    if (l.event === 'refund_requested') return `操作人 #${d.actor_id} · 退款凭证 ${d.refund_reference} · ${d.reason}`
    if (l.event === 'recheck_requested' || l.event === 'issue_resolved') return `操作人 #${d.actor_id} · ${d.reason}`
  } catch { /* Plain-text events need no parsing. */ }
  return l.detail
}
function showError(e: any) { error.value = e?.response?.data?.message || e?.message || '请求失败，请稍后重试' }
async function refresh() {
  const [me, list, history] = await Promise.all([apiClient.get('/proxy/me'), apiClient.get('/recharge/orders'), apiClient.get('/recharge/logs')])
  isAdmin.value = me.data.role === 'admin'; balance.value = me.data.balance; email.value = me.data.email; authenticated.value = true
  orders.value = list.data; logs.value = history.data
}
const checkoutOpen = ref(false), selectedProduct = ref<Package | null>(null), selectedOrder = ref<Order | null>(null)
function create(p: Package) {
 if(!authenticated.value){error.value='请先在本站登录后再充值';return}
 selectedProduct.value=p;selectedOrder.value=null;checkoutOpen.value=true
}
function resume(o: Order) {selectedOrder.value=o;selectedProduct.value=null;checkoutOpen.value=true}
async function paymentCompleted() {notice.value='支付成功，余额已自动到账';try{await refresh()}catch{error.value='余额已入账，页面信息暂未刷新，请稍后刷新查看。'}}
async function closeCheckout() {checkoutOpen.value=false;try{await refresh()}catch(e){showError(e)}}
async function checkReturn(id: string) {
  let attempts = 0
  const check = async () => {
    try {
      const { data } = await apiClient.get(`/recharge/orders/${encodeURIComponent(id)}`)
      if (data.order.reversed_at) { notice.value = '该订单已登记退款并完成余额冲正'; clearInterval(timer); await refresh() }
      else if (data.order.status === 'paid') { notice.value = '支付成功，余额已自动到账'; clearInterval(timer); await refresh() }
      else if (++attempts >= 60) { clearInterval(timer); notice.value = '支付确认仍在处理中，请稍后刷新订单。' }
    } catch (e) { clearInterval(timer); showError(e) }
  }
  timer = setInterval(check, 3000); await check()
}
onMounted(async () => {
  try { packages.value = (await apiClient.get('/recharge/packages')).data } catch (e) { showError(e) }
  const token = localStorage.getItem('auth_token') || localStorage.getItem('token')
  if (!token) return
  try {
    await refresh()
    const id = new URLSearchParams(window.location.search).get('order')
    if (id) { tab.value = 'orders'; notice.value = '正在等待支付结果确认…'; await checkReturn(id) }
  } catch (e) { showError(e) }
})
onUnmounted(() => clearInterval(timer))
async function recheck(o: Order) {
  busy.value = true; error.value = ''
  try {
    const { data } = await apiClient.post(`/recharge/orders/${encodeURIComponent(o.id)}/recheck`)
    notice.value = data.status === 'paid' ? '付款已确认，余额已到账' : '支付平台尚未确认付款，请稍后再查'
    await refresh()
  } catch (e) { showError(e) } finally { busy.value = false }
}
const events: Record<string, string> = { credited: '余额已入账', created: '订单创建', exception: '订单异常', recheck_requested: '申请查单', issue_resolved: '异常已处理', refund_registered: '退款已冲正', refund_requested: '申请退款登记' }
async function manualRefresh() { error.value = ''; try { await refresh() } catch (e) { showError(e) } }
</script>

<template>
  <div class="recharge">
    <header><div class="nav"><a class="brand" href="/fastaiui/recharge"><i></i>自动充值</a><nav><button :class="{ active: tab === 'products' }" @click="tab = 'products'">充值套餐</button><button :class="{ active: tab === 'orders' }" @click="tab = 'orders'">充值记录</button><button :class="{ active: tab === 'logs' }" @click="tab = 'logs'">到账日志</button><button v-if="isAdmin" :class="{ active: tab === 'admin' }" @click="tab = 'admin'">充值管理</button></nav><a class="back" href="/fastaiui/">返回绘画</a></div></header>
    <main>
      <section class="hero"><span class="eyebrow">API BALANCE</span><h1>余额随时续航</h1><p>选择额度，完成支付，余额自动到账。</p><div class="account"><template v-if="authenticated"><span>{{ email }}</span><span>当前余额 <strong>${{ balance }}</strong></span></template><template v-else><span>登录后即可充值并查看到账记录</span><a href="/login">前往登录 →</a></template></div></section>
      <div v-if="error" class="alert error" role="alert">{{ error }}<button @click="error = ''">关闭</button></div>
      <div v-if="notice" class="alert" role="status">{{ notice }}</div>
      <template v-if="tab === 'products'">
        <div class="toolbar"><h2>精选充值套餐</h2><span class="supported-methods">支持支付宝 · 微信支付</span></div>
        <div class="products"><article v-for="p in packages" :key="p.id" class="card"><h3>{{ p.name }}</h3><p>{{ Number(p.credit) }} USD API 使用额度</p><div class="badges"><span>ϟ 自动到账</span><span class="green">● 官方余额直充</span></div><div class="price"><div>¥{{ amount(p.amount_cents) }} <small>CNY</small></div><button :disabled="busy || !authenticated" @click="create(p)">立即充值 <span>↗</span></button></div></article></div>
        <p class="hint">到账额度以套餐显示为准。完成支付后，可在充值记录中查看到账进度。</p>
      </template>
      <RechargeAdmin v-else-if="tab === 'admin' && isAdmin"/>
      <template v-else>
        <div class="toolbar"><h2>{{ tab === 'orders' ? '充值记录' : '到账日志' }}</h2><button class="refresh" @click="manualRefresh">刷新记录 ↻</button></div>
        <div v-if="!authenticated" class="empty">请先登录以查看您的充值记录。</div>
        <div v-else-if="tab === 'orders'" class="table-wrap"><table><thead><tr><th>订单号 / 时间</th><th>支付金额</th><th>到账额度</th><th>状态</th><th></th></tr></thead><tbody><tr v-for="o in orders" :key="o.id"><td><code>{{ o.id }}</code><small>{{ time(o.created_at) }}</small></td><td>¥{{ amount(o.amount_cents) }}</td><td>${{ Number(o.credit) }}</td><td><span class="status" :class="{ paid: o.status === 'paid' }">{{ o.reversed_at ? '退款已冲正' : o.status === 'paid' ? '已到账' : o.gateway_status === 'expired' ? '平台超时' : '待支付' }}</span></td><td><button v-if="o.status === 'pending'" :disabled="busy" class="refresh" @click="recheck(o)">我已付款，查到账</button> <button v-if="o.status === 'pending' && o.gateway_status !== 'expired'" :disabled="busy" class="refresh" @click="resume(o)">继续支付</button></td></tr></tbody></table><div v-if="!orders.length" class="empty">还没有充值记录，选一个套餐开始吧。</div></div>
        <div v-else class="table-wrap"><table><thead><tr><th>时间</th><th>订单号</th><th>事件</th><th>详情</th></tr></thead><tbody><tr v-for="l in logs" :key="l.order_id + l.event + l.created_at"><td>{{ time(l.created_at) }}</td><td><code>{{ l.order_id }}</code></td><td>{{ events[l.event] || l.event }}</td><td>{{ logDetail(l) }}</td></tr></tbody></table><div v-if="!logs.length" class="empty">暂无到账日志。</div></div>
        <p class="hint">显示最近 100 条记录。</p>
      </template>
    </main><RechargeDialog v-if="checkoutOpen" :product="selectedProduct" :existing-order="selectedOrder" :email="email" @close="closeCheckout" @paid="paymentCompleted"/><footer>安全支付 · 自动到账 · 每笔可追溯</footer>
  </div>
</template>

<style scoped>
.recharge{min-height:100vh;background:#fafafa;color:#191a20;font-family:Inter,-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}header{background:#fff;border-bottom:1px solid #e9e9ee}.nav{max-width:1280px;margin:auto;display:flex;align-items:center;gap:48px;min-height:80px;padding:0 24px}a{text-decoration:none;color:inherit}.brand{display:flex;align-items:center;gap:9px;font-weight:750;color:#2764ed;background:#eff6ff;border:1px solid #bfd7ff;border-radius:30px;padding:11px 18px}.brand i{width:9px;height:9px;border-radius:50%;background:#2764ed}nav{display:flex;gap:24px}nav button{border:0;background:none;color:#81838f;padding:12px 0}nav .active{color:#20232c;font-weight:650}.back{margin-left:auto;font-size:14px;color:#686c78}main{max-width:1280px;margin:auto;padding:0 24px}.hero{text-align:center;padding:62px 0 46px}.eyebrow{color:#2864e8;font-size:12px;letter-spacing:3px;font-weight:650}h1{font-size:48px;letter-spacing:-2px;margin:15px 0}p{color:#858590;line-height:1.6}.hero>p{font-size:17px}.account{display:inline-flex;gap:24px;align-items:center;background:#fff;border:1px solid #e8e8ee;border-radius:12px;padding:13px 20px;font-size:13px;color:#777b87;margin-top:14px}.account strong{color:#1d2434;margin-left:6px}.account a{color:#2864e8}.toolbar{display:flex;align-items:center;justify-content:space-between;margin:12px 0 24px;gap:15px}h2{font-size:19px;margin:0}.supported-methods{font-size:13px;color:#8a909b}.payment{display:flex;gap:20px;background:white;border:1px solid #e8e8ee;border-radius:30px;padding:12px 18px;font-size:14px}.payment label{display:flex;align-items:center;gap:6px;cursor:pointer}input{accent-color:#2864e8}.products{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:18px}.card{background:#fff;border:1px solid #e5e6ec;border-radius:19px;padding:26px 23px;box-shadow:0 2px 3px #181c2905}.card h3{font-size:23px;margin:5px 0 16px;letter-spacing:-.6px}.card p{font-size:13px;margin:0 0 18px}.badges{display:flex;gap:7px;flex-wrap:wrap;min-height:53px;align-content:flex-start}.badges span{font-size:11px;border:1px solid #c4dcff;border-radius:20px;background:#f0f7ff;color:#3071ef;padding:5px 9px}.badges .green{background:#f1fcf5;border-color:#c4f0d2;color:#22a05e}.price{display:flex;align-items:center;justify-content:space-between;gap:8px;border-top:1px dashed #e6e7ed;margin-top:14px;padding-top:20px}.price>div{font-size:23px;font-weight:750;letter-spacing:-1px;white-space:nowrap}.price small{font-size:10px;color:#a2a5af;letter-spacing:0}.price button{border:0;border-radius:24px;background:#1b1b20;color:#fff;font-size:12px;font-weight:600;padding:11px 12px;white-space:nowrap}.price button span{margin-left:6px}button:disabled{opacity:.4;cursor:not-allowed}.hint{font-size:12px;margin:24px 0 40px}.alert{display:flex;justify-content:space-between;padding:15px 20px;border:1px solid #bed8fb;background:#eef6ff;color:#2864ad;border-radius:10px;margin-bottom:20px;font-size:14px}.error{background:#fff2f2;color:#ad3333;border-color:#facaca}.alert button{border:0;background:transparent;color:inherit}.table-wrap{overflow-x:auto;border:1px solid #e5e6ec;border-radius:14px;background:#fff}table{width:100%;border-collapse:collapse;text-align:left;font-size:13px}th{background:#f5f6f9;color:#858895;font-weight:500;white-space:nowrap}td,th{padding:19px 20px;border-bottom:1px solid #eeeef2}td small{display:block;color:#9596a1;margin-top:6px}td code{font-size:11px}td:last-child{max-width:350px;overflow-wrap:anywhere}.status{display:inline-block;white-space:nowrap;border-radius:20px;padding:5px 10px;background:#fff7e6;color:#ad7a20}.paid{background:#eaf9f0;color:#22985c}.refresh{background:#fff;border:1px solid #e1e3ea;border-radius:8px;font-size:12px;padding:9px 13px;white-space:nowrap}.empty{padding:50px;text-align:center;color:#9395a0}footer{text-align:center;color:#a0a2ad;font-size:12px;padding:20px 0 35px}@media(max-width:1100px){.products{grid-template-columns:repeat(2,minmax(0,1fr))}}@media(max-width:600px){.nav{gap:16px;flex-wrap:wrap;padding:16px;min-height:70px}nav{gap:16px;order:3;width:100%;justify-content:center;font-size:14px}.hero{padding:36px 0 26px}h1{font-size:36px}.account{flex-direction:column;gap:8px}.products{grid-template-columns:1fr}.toolbar{flex-direction:column;align-items:flex-start}main{padding:0 16px}.card{padding:24px}.badges{min-height:35px}}
</style>
