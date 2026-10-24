<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { apiClient } from '../../api/client'
import RechargeAdmin from './RechargeAdmin.vue'
import RechargeDialog from './RechargeDialog.vue'
import RechargeOrders from './RechargeOrders.vue'
import RechargeLogs from './RechargeLogs.vue'
import type {Package,Order,Log} from './types'
const packages = ref<Package[]>([]), orders = ref<Order[]>([]), logs = ref<Log[]>([])
const tab = ref('products'), busy = ref(false), error = ref(''), notice = ref('')
const balance = ref<string | number>('—'), email = ref(''), authenticated = ref(false), isAdmin = ref(false)
let timer: ReturnType<typeof setInterval> | undefined, refreshTimer: ReturnType<typeof setInterval> | undefined
const paidOrders = computed(() => orders.value.filter(o => o.status === 'paid' && !o.reversed_at))
const totalCredit = computed(() => paidOrders.value.reduce((n,o) => n+Number(o.credited_credit ?? o.credit),0))
const pendingCount = computed(() => orders.value.filter(o => o.status === 'pending' && o.gateway_status !== 'expired').length)
const amount = (n: number) => (n / 100).toFixed(2)
function showError(e: any) { error.value = e?.response?.data?.message || e?.message || '请求失败，请稍后重试' }
let refreshSequence=0
async function refresh() {
 const sequence=++refreshSequence, token=localStorage.getItem('auth_token') || localStorage.getItem('token')
 let me
 try {me=await apiClient.get('/proxy/me')} catch(e:any) {
  if(sequence===refreshSequence && (e?.response?.status===401 || e?.status===401)){authenticated.value=false;isAdmin.value=false;orders.value=[];logs.value=[];email.value='';balance.value='—'}
  throw e
 }
 const [list,history,access]=await Promise.allSettled([apiClient.get('/recharge/orders'),apiClient.get('/recharge/logs'),apiClient.get('/recharge/access')])
 if(sequence!==refreshSequence || token!==(localStorage.getItem('auth_token') || localStorage.getItem('token')))return
 if(email.value!==me.data.email){orders.value=[];logs.value=[];isAdmin.value=false}
 balance.value=me.data.balance;email.value=me.data.email;authenticated.value=true
 if(list.status==='fulfilled')orders.value=list.value.data;else showError(list.reason)
 if(history.status==='fulfilled')logs.value=history.value.data;else showError(history.reason)
 isAdmin.value=access.status==='fulfilled' && access.value.data.is_admin===true
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
    refreshTimer=setInterval(() => {if(document.visibilityState==='visible') refresh().catch(showError)},30000)
    const id = new URLSearchParams(window.location.search).get('order')
    if (id) { tab.value = 'orders'; notice.value = '正在等待支付结果确认…'; await checkReturn(id) }
  } catch (e) { showError(e) }
})
onUnmounted(() => {clearInterval(timer);clearInterval(refreshTimer)})
async function recheck(o: Order) {
  busy.value = true; error.value = ''
  try {
    const { data } = await apiClient.post(`/recharge/orders/${encodeURIComponent(o.id)}/recheck`)
    notice.value = data.status === 'paid' ? '付款已确认，余额已到账' : '支付平台尚未确认付款，请稍后再查'
    await refresh()
  } catch (e) { showError(e) } finally { busy.value = false }
}
async function manualRefresh() { error.value = ''; try { await refresh() } catch (e) { showError(e) } }
</script>

<template>
  <div class="recharge">
    <header><div class="nav"><a class="brand" href="/fastaiui/recharge"><i>↗</i> 充值中心</a><nav aria-label="充值导航"><button :class="{ active: tab === 'products' }" @click="tab = 'products'">总览</button><button :class="{ active: tab === 'orders' }" @click="tab = 'orders'">充值记录</button><button :class="{ active: tab === 'logs' }" @click="tab = 'logs'">到账日志</button><button v-if="isAdmin" :class="{ active: tab === 'admin' }" @click="tab = 'admin'">充值管理 <small>管理员</small></button></nav><a v-if="isAdmin" class="back" href="/fastaiui/admin/client-releases">客户端发布 ↗</a><a class="back" href="/fastaiui/">返回绘画 ↗</a></div></header>
    <main>
      <section class="page-heading"><div><h1>自动充值</h1><p>选择套餐，按订单金额扫码支付，渠道确认后自动到账。</p></div><div class="identity" v-if="authenticated"><span>{{ email }}</span><span class="role">{{ isAdmin ? '管理员账号' : '仅显示我的数据' }}</span><button class="refresh" @click="manualRefresh">刷新 ↻</button></div><a v-else class="login" href="/login">登录后充值 →</a></section>
      <div v-if="error" class="alert error" role="alert">{{ error }}<button @click="error = ''">关闭</button></div><div v-if="notice" class="alert" role="status">{{ notice }}<button @click="notice = ''">关闭</button></div>
      <section class="metrics"><div><span>当前可用余额</span><strong>${{ balance }}</strong></div><div><span>最近订单到账额度</span><strong>${{ Number(totalCredit.toFixed(8)) }}</strong></div><div><span>已到账订单</span><strong>{{ paidOrders.length }} <small>笔</small></strong></div><div><span>待支付订单</span><strong>{{ pendingCount }} <small>笔</small></strong></div></section>
      <template v-if="tab === 'products'">
        <section class="panel packages-panel"><div class="panel-heading"><h2>选择充值套餐</h2><span>支付宝 / 微信 · 服务端核验 · 自动到账</span></div><div class="products"><article v-for="p in packages" :key="p.id" class="package"><div><strong>${{ Number(p.credit) }} <small>API 额度</small></strong><span>支付 ¥{{ amount(p.amount_cents) }}</span></div><button :disabled="busy || !authenticated" :aria-label="`立即充值 ${p.name}`" @click="create(p)">立即充值 ↗</button></article></div><p class="payment-warning" role="note"><strong>支付金额必须与下单金额一致，否则无法自动到账。</strong>请勿修改扫码后的付款金额；已支付但未到账，请保留交易单号并联系管理员核对，勿重复付款。</p></section>
        <div class="workspace"><section class="panel"><div class="panel-heading"><h2>我的充值记录 <b>{{ orders.length }}</b></h2><button class="text-button" @click="tab='orders'">查看记录 →</button></div><RechargeOrders :orders="orders" :busy="busy" :page-size="3" @resume="resume" @recheck="recheck"/></section><section class="panel"><div class="panel-heading"><h2>我的到账日志 <b>{{ logs.length }}</b></h2><button class="text-button" @click="tab='logs'">查看日志 →</button></div><RechargeLogs :logs="logs" :page-size="3"/></section></div>
      </template>
      <RechargeAdmin v-else-if="tab === 'admin' && isAdmin"/>
      <section v-else class="panel"><div class="panel-heading"><h2>{{ tab === 'orders' ? '我的充值记录' : '我的到账日志' }}</h2><span>仅当前账号 · 最近 100 条 · 分页展示</span></div><p v-if="!authenticated" class="empty">请先登录以查看自己的记录。</p><RechargeOrders v-else-if="tab==='orders'" :orders="orders" :busy="busy" :page-size="8" @resume="resume" @recheck="recheck"/><RechargeLogs v-else :logs="logs" :page-size="8"/></section>
      <footer>渠道确认订单支付成功后自动入账。普通用户仅可查看自己的订单；充值管理仅管理员可用。<span>技术交流 QQ 群：48878665</span></footer>
    </main><RechargeDialog v-if="checkoutOpen" :product="selectedProduct" :existing-order="selectedOrder" :email="email" @close="closeCheckout" @paid="paymentCompleted"/>
  </div>
</template>
<style scoped>
.recharge{min-height:100vh;background:#f6f8fc;color:#243047;color-scheme:light;font-family:Inter,-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}.recharge button{font:inherit;cursor:pointer}.recharge button:disabled{opacity:.45;cursor:not-allowed}a{text-decoration:none;color:inherit}header{background:#fff;border-bottom:1px solid #e6ebf3}.nav{max-width:1440px;margin:auto;display:flex;align-items:center;gap:30px;height:54px;padding:0 28px}.brand{font-size:17px;font-weight:750;white-space:nowrap}.brand i{display:inline-grid;place-items:center;width:28px;height:28px;background:#2864ed;border-radius:8px;color:#fff;font-style:normal}nav{display:flex;height:100%;align-items:center;gap:6px}nav button{border:0;background:transparent;color:#6a7890;padding:10px 14px;border-radius:8px;font-size:13px!important}nav button.active{background:#edf3ff;color:#285ee5;font-weight:650}nav small{font-size:10px;margin-left:3px}.back{margin-left:auto;font-size:12px;color:#6a7890;white-space:nowrap}main{max-width:1440px;padding:16px 28px 0;margin:auto}.page-heading{display:flex;justify-content:space-between;align-items:center;gap:18px;margin-bottom:12px}h1{font-size:22px;letter-spacing:-.5px;margin:0 0 5px}.page-heading p{font-size:12px;color:#7c879b;margin:0}.identity{display:flex;align-items:center;gap:10px;font-size:12px;color:#66748b}.role{border:1px solid #dce5f5;background:white;padding:5px 8px;border-radius:6px;font-size:10px}.refresh{color:#4e607e;background:white;border:1px solid #dce4f0;border-radius:7px;padding:7px 12px;font-size:12px!important}.login{color:#2864ed;font-size:13px}.metrics{display:grid;grid-template-columns:1.4fr 1.4fr 1fr 1fr;gap:12px;margin-bottom:12px}.metrics>div{background:#fff;border:1px solid #e3e9f2;border-radius:11px;padding:10px 16px;display:flex;flex-direction:column;gap:5px}.metrics span{font-size:11px;color:#8590a2}.metrics strong{font-size:23px;letter-spacing:-.5px;color:#243047}.metrics>div:first-child strong{color:#275ee7}.metrics small{font-size:11px;font-weight:400;color:#8e99aa}.panel{background:#fff;border:1px solid #e3e9f2;border-radius:12px;min-width:0;overflow:hidden}.panel-heading{padding:12px 16px;display:flex;align-items:center;justify-content:space-between;gap:10px;border-bottom:1px solid #edf0f6}.panel-heading h2{font-size:14px;margin:0;color:#2d3b53}.panel-heading h2 b{font-size:10px;background:#edf2fa;color:#71809a;padding:3px 6px;border-radius:5px;font-weight:500;margin-left:4px}.panel-heading>span{font-size:11px;color:#8b97a8}.text-button{border:0;background:transparent;color:#5272ae;padding:0;font-size:11px!important}.products{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:8px;padding:10px 14px 8px}.package{display:flex;align-items:center;justify-content:space-between;border:1px solid #e4eaf4;border-radius:9px;padding:9px 12px;gap:8px}.package:hover{border-color:#94b4f5;background:#f9fbff}.package>div{display:flex;flex-direction:column;gap:6px}.package strong{font-size:20px;color:#27364e}.package small{display:block;font-size:10px;font-weight:400;color:#8390a4;margin-top:3px}.package span{font-size:11px;color:#6c7c94}.package button{font-size:11px!important;border:0;border-radius:7px;padding:9px 10px;background:#edf3ff;color:#2c62df;white-space:nowrap}.payment-warning{margin:0 14px 10px;padding:9px 12px;border:1px solid #fecaca;border-radius:8px;background:#fff1f2;color:#b91c1c;font-size:12px;line-height:1.6}.payment-warning strong{font-weight:700}.workspace{display:grid;grid-template-columns:1.2fr 1fr;gap:14px;margin-top:12px}.alert{display:flex;justify-content:space-between;align-items:center;border:1px solid #cae0ff;background:#edf5ff;color:#356398;border-radius:8px;font-size:12px;padding:10px 14px;margin:0 0 12px;gap:12px}.alert button{color:inherit;border:0;background:transparent;font-size:11px}.error{background:#fff1ef;border-color:#f5d1cc;color:#b14a3c}.empty{color:#8997aa;text-align:center;padding:36px;font-size:13px}footer{font-size:10px;color:#919daf;display:flex;justify-content:space-between;gap:14px;padding:15px 0 18px;line-height:1.6}@media(min-width:1500px){main{padding-top:24px}.products{gap:12px}.package{padding:15px}}@media(max-width:1050px){.workspace{grid-template-columns:1fr}.nav{gap:15px}.identity .role{display:none}.products{grid-template-columns:repeat(4,minmax(0,1fr))}.package{flex-direction:column;align-items:stretch;gap:10px}.package small{display:inline;margin-left:4px}}@media(max-width:680px){main{padding:15px 14px 0}.nav{height:auto;min-height:54px;flex-wrap:wrap;padding:12px 14px;gap:10px}nav{order:3;width:100%;justify-content:space-between}nav button{padding:8px!important;font-size:12px!important}.page-heading{align-items:flex-start;flex-direction:column;gap:10px}.identity{width:100%;justify-content:space-between}.metrics{grid-template-columns:1fr 1fr;gap:8px}.metrics>div{padding:12px}.metrics strong{font-size:19px}.products{grid-template-columns:repeat(2,minmax(0,1fr));padding:12px;gap:8px}.package{flex-direction:row;align-items:center;padding:12px 10px}.package small{display:block;margin-left:0}.panel-heading{padding:13px}.panel-heading>span{max-width:50%;text-align:right}.workspace{margin-top:12px;gap:12px}footer{flex-direction:column;gap:4px}h1{font-size:22px}}
@media(min-width:1100px) and (max-height:850px){.products{grid-template-columns:repeat(8,minmax(0,1fr))}.package{flex-direction:column;align-items:stretch;gap:7px;padding:10px}.package small{display:inline;font-size:9px;margin-left:3px}.package strong{font-size:18px}.page-heading{margin-bottom:10px}.metrics{margin-bottom:10px}}
</style>
