<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import QRCode from 'qrcode'
import { apiClient } from '../../api/client'
import type { Package, Order, Checkout } from './types'
const props = defineProps<{ product?: Package | null; existingOrder?: Order | null; email: string }>()
const emit = defineEmits<{ close: []; paid: [order: Order] }>()
const channel = ref(props.existingOrder?.payment_type || 'alipay'), order = ref<Order | null>(props.existingOrder || null)
const phase = ref<'confirm' | 'loading' | 'qr' | 'waiting' | 'success' | 'expired'>('confirm')
const attempted = ref(false)
const submitting = ref(false), checking = ref(false), error = ref(''), qrImage = ref(''), paused = ref(false)
const modal = ref<HTMLElement | null>(null), requestID = crypto.randomUUID(), controller = new AbortController()
let timer: ReturnType<typeof setTimeout> | undefined, disposed = false, paidEmitted = false, polling = false, pollCount = 0
let previousFocus: Element | null, previousOverflow = ''
const amount = computed(() => ((order.value?.amount_cents ?? props.product?.amount_cents ?? 0) / 100).toFixed(2))
const credit = computed(() => Number(order.value?.credited_credit ?? order.value?.credit ?? props.product?.credit ?? 0))
const title = computed(() => props.product?.name || `${credit.value} 刀额度`)
const channelName = computed(() => channel.value === 'wxpay' ? '微信' : '支付宝')
function message(e: any) { return e?.response?.data?.message || e?.message || '网络暂时不可用，请稍后重试' }
function close() { if (!submitting.value) emit('close') }
function stopPolling() { polling = false; clearTimeout(timer) }
async function apply(session: Checkout) {
  if (disposed) return
  order.value = session.order; channel.value = session.order.payment_type
  if (session.order.reversed_at || session.state === 'expired' || session.order.gateway_status === 'expired') { phase.value = 'expired'; stopPolling(); return }
  if (session.order.status === 'paid') {
    phase.value = 'success'; error.value = ''; stopPolling()
    if (!paidEmitted) { paidEmitted = true; emit('paid', session.order) }
    return
  }
  if (session.state === 'ready' && session.qr_code) {
    const image = session.qr_kind === 'image' ? session.qr_code : await QRCode.toDataURL(session.qr_code, { width: 280, margin: 2, errorCorrectionLevel: 'M' })
    if (disposed) return
    qrImage.value = image; phase.value = 'qr'; error.value = ''
  } else if (!qrImage.value) phase.value = 'waiting'
}
function startPolling() {
  if (polling || !order.value || phase.value === 'success' || phase.value === 'expired') return
  polling = true; pollCount = 0; paused.value = false
  const tick = async () => {
    if (!polling || disposed || !order.value) return
    try {
      const { data } = await apiClient.get<Checkout>(`/recharge/orders/${encodeURIComponent(order.value.id)}`, { signal: controller.signal, timeout: 12000 })
      await apply(data)
    } catch (e: any) {
      if (disposed) return
      if ((e?.response?.status || e?.status) === 401) { error.value = '登录已过期，请回主站重新登录，再从充值记录继续支付。'; stopPolling(); return }
      error.value = '暂时无法查询到账状态，正在重试；请勿重复付款。'
    }
    if (++pollCount >= 200) { paused.value = true; stopPolling(); return }
    if (polling) timer = setTimeout(tick, 3000)
  }
  timer = setTimeout(tick, 3000)
}
async function submit() {
  if (submitting.value || disposed) return
  attempted.value = true; submitting.value = true; error.value = ''; phase.value = 'loading'
  try {
    if (!order.value) {
      const { data } = await apiClient.post<{ order: Order }>('/recharge/orders', { package_id: props.product!.id, payment_type: channel.value, request_id: requestID }, { signal: controller.signal, timeout: 20000 })
      order.value = data.order
    }
    const { data } = await apiClient.post<Checkout>(`/recharge/orders/${encodeURIComponent(order.value.id)}/checkout`, {}, { signal: controller.signal, timeout: 35000 })
    await apply(data); startPolling()
  } catch (e) {
    if (disposed) return
    phase.value = order.value ? 'waiting' : 'confirm'
    error.value = order.value ? message(e) : '提交结果暂未确认，重试会沿用相同请求，也可稍后在充值记录中查看。'
    if (order.value) startPolling()
  } finally { if (!disposed) submitting.value = false }
}
async function checkPayment() {
  if (!order.value || checking.value) return
  checking.value = true; error.value = ''
  try {
    const { data } = await apiClient.post<Order>(`/recharge/orders/${encodeURIComponent(order.value.id)}/recheck`, {}, { signal: controller.signal, timeout: 20000 })
    await apply({ order: data, state: data.status })
    if (data.status !== 'paid') error.value = '支付结果仍在确认中，请稍候；已付款请勿重复扫码。'
    if (paused.value) startPolling()
  } catch (e) { if (!disposed) error.value = message(e) }
  finally { if (!disposed) checking.value = false }
}
function keydown(e: KeyboardEvent) {
  if (e.key === 'Escape') { e.preventDefault(); close() }
  if (e.key !== 'Tab' || !modal.value) return
  const elements = Array.from(modal.value.querySelectorAll<HTMLElement>('button:not(:disabled),input:not(:disabled),[tabindex="0"]'))
  const first = elements[0], last = elements[elements.length - 1]
  if (e.shiftKey && (document.activeElement === first || document.activeElement === modal.value)) { e.preventDefault(); last?.focus() }
  else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first?.focus() }
}
onMounted(async () => {
  previousFocus = document.activeElement; previousOverflow = document.body.style.overflow; document.body.style.overflow = 'hidden'
  await nextTick(); modal.value?.focus(); if (props.existingOrder) await submit()
})
onUnmounted(() => {
  disposed = true; stopPolling(); controller.abort(); document.body.style.overflow = previousOverflow
  if (previousFocus instanceof HTMLElement && previousFocus.isConnected) previousFocus.focus()
})
</script>
<template>
  <Teleport to="body"><div class="checkout-backdrop" @click.self="close">
    <section ref="modal" class="checkout-dialog" role="dialog" aria-modal="true" aria-labelledby="checkout-title" tabindex="-1" @keydown="keydown">
      <button class="close" aria-label="关闭充值窗口" :disabled="submitting" @click="close">×</button>
      <template v-if="phase === 'confirm'">
        <span class="eyebrow">确认充值</span><h2 id="checkout-title">购买 · {{ title }}</h2>
        <div class="account"><span>充值账号</span><strong>{{ email }}</strong></div>
        <div class="summary"><div><span>到账额度</span><strong>${{ credit }} <small>USD</small></strong></div><div><span>支付金额</span><strong class="amount">¥{{ amount }}</strong></div></div>
        <fieldset><legend>选择支付方式</legend>
          <label :class="{ selected: channel === 'alipay' }"><input v-model="channel" :disabled="attempted" type="radio" name="checkout-channel" value="alipay"><i class="alipay">支</i><span>支付宝</span><b>{{ channel === 'alipay' ? '●' : '○' }}</b></label>
          <label :class="{ selected: channel === 'wxpay' }"><input v-model="channel" :disabled="attempted" type="radio" name="checkout-channel" value="wxpay"><i class="wxpay">微</i><span>微信支付</span><b>{{ channel === 'wxpay' ? '●' : '○' }}</b></label>
        </fieldset>
        <p class="payment-warning" role="note"><strong>支付金额必须与下单金额一致，否则无法自动到账。</strong>请按 ¥{{ amount }} 支付，勿修改付款金额。</p>
        <div v-if="error" class="error" role="alert">{{ error }}</div>
        <button class="primary" :disabled="submitting" @click="submit">提交订单</button>
      </template>
      <template v-else-if="phase === 'success'">
        <div class="result"><div class="success-icon">✓</div><h2 id="checkout-title">充值成功</h2><p>实付 ¥{{ ((order?.paid_amount_cents ?? order?.amount_cents ?? 0) / 100).toFixed(2) }}，<strong>${{ credit }} USD</strong> 额度已自动到账。</p><code>{{ order?.id }}</code><button class="primary" @click="close">完成</button></div>
      </template>
      <template v-else-if="phase === 'expired'">
        <div class="result"><h2 id="checkout-title">{{ order?.reversed_at ? '订单已退款冲正' : '二维码已过期' }}</h2><p>请关闭窗口，在充值记录中查看该订单。</p><code>{{ order?.id }}</code><button class="primary" @click="close">关闭窗口</button></div>
      </template>
      <template v-else>
        <span class="eyebrow">{{ channelName }}扫码支付</span><h2 id="checkout-title">{{ phase === 'loading' ? '正在生成订单…' : '请扫码完成充值' }}</h2>
        <div class="scan-amount">¥{{ amount }}<small>到账 ${{ credit }} USD</small></div>
        <p class="payment-warning" role="note"><strong>支付金额必须与下单金额一致，否则无法自动到账。</strong>请按 ¥{{ amount }} 支付，勿修改付款金额。</p>
        <div v-if="qrImage && phase !== 'loading'" class="qr"><img :src="qrImage" :alt="`${channelName}支付二维码`" width="256" height="256"></div>
        <div v-else class="qr placeholder"><span class="spinner" v-if="submitting"></span><span>{{ submitting ? '正在获取渠道二维码' : '订单正在确认，请稍后重试获取二维码' }}</span></div>
        <p class="scan-tip">{{ qrImage ? `请使用${channelName}扫一扫，按显示金额付款` : '将保留原订单，不会因重试重复创建渠道订单。' }}</p>
        <div v-if="order" class="order"><span>订单号</span><code>{{ order.id }}</code></div>
        <p v-if="paused" class="footnote">自动查询已暂停，可点击下方按钮重新确认。</p>
        <div v-if="error" class="error" role="alert">{{ error }}</div>
        <button v-if="!qrImage" class="primary" :disabled="submitting" @click="submit">{{ submitting ? '处理中…' : '重试获取二维码' }}</button>
        <button v-else class="primary" :disabled="checking || submitting" @click="checkPayment">{{ checking ? '正在确认…' : '我已付款，确认到账' }}</button>
        <p class="footnote">渠道确认收款后自动到账。已付款未到账，请保留交易单号联系管理员，勿重复付款。</p>
      </template>
    </section>
  </div></Teleport>
</template>
<style scoped>
.checkout-backdrop{position:fixed;inset:0;z-index:1000;background:#10131c75;backdrop-filter:blur(3px);display:flex;align-items:center;justify-content:center;padding:24px;color:#191a20;color-scheme:light}.checkout-dialog{position:relative;width:520px;max-width:100%;max-height:calc(100dvh - 48px);overflow:auto;background:#fff;border-radius:25px;padding:36px;box-shadow:0 24px 100px #0d112733;outline:none}.close{position:absolute;right:22px;top:22px;width:36px;height:36px;display:grid;place-items:center;border:0;border-radius:50%;background:#f4f4f6;color:#82848e;font-size:29px;line-height:1}.eyebrow{display:block;font-size:12px;font-weight:600;color:#858c9b;margin:2px 0 10px}h2{font-size:27px;letter-spacing:-.6px;margin:0 35px 28px 0}.account{display:flex;flex-direction:column;gap:8px;font-size:13px;margin-bottom:24px;color:#8c909a}.account strong{font-size:15px;font-weight:500;color:#4e5462;overflow-wrap:anywhere}.summary{background:#f6f7fa;border-radius:13px;padding:20px;margin-bottom:26px}.summary>div{display:flex;justify-content:space-between;align-items:center;gap:12px;font-size:14px;color:#757a86}.summary>div+div{margin-top:18px}.summary strong{color:#252833;font-size:19px}.summary small{font-size:11px;color:#969ba5}.summary .amount{font-size:29px;color:#17191f;font-weight:750}fieldset{border:0;padding:0;margin:0 0 16px;display:grid;grid-template-columns:1fr 1fr;gap:12px}legend{font-size:14px;font-weight:650;margin-bottom:14px;padding:0}label{display:flex;align-items:center;gap:9px;border:1px solid #e2e4eb;border-radius:12px;padding:16px 12px;cursor:pointer;font-size:14px}label.selected{background:#f1f6ff;border-color:#72a0fa}input{position:absolute;opacity:0;width:1px;height:1px}label:focus-within{outline:2px solid #367af1;outline-offset:3px}i{width:28px;height:28px;border-radius:7px;display:grid;place-items:center;color:white;font-weight:650;font-style:normal}.alipay{background:#1b99e7}.wxpay{background:#20b968}label b{margin-left:auto;color:#9ca2af;font-size:18px}.selected b{color:#367af1}.primary{width:100%;border:0;border-radius:12px;background:#191a1f;color:#fff;padding:17px;font-size:16px;font-weight:650;cursor:pointer}button:disabled{opacity:.45;cursor:not-allowed}.footnote{font-size:12px;color:#9398a4;line-height:1.7;text-align:center;margin:17px 0}.payment-warning{padding:10px 12px;border:1px solid #fecaca;border-radius:9px;background:#fff1f2;color:#b91c1c;font-size:12px;line-height:1.7;margin:14px 0}.payment-warning strong{display:block;font-weight:700}.error{padding:12px 15px;background:#fff2f0;color:#b64e3f;border:1px solid #f6d5cf;border-radius:9px;font-size:13px;line-height:1.7;margin:14px 0}.scan-amount{text-align:center;font-size:36px;font-weight:750;letter-spacing:-1px;margin:0 0 18px}.scan-amount small{display:block;color:#9299a5;font-size:12px;letter-spacing:0;font-weight:400;margin-top:6px}.qr{width:280px;height:280px;max-width:100%;margin:0 auto;background:#fff;border:1px solid #e7e9ef;border-radius:15px;display:flex;align-items:center;justify-content:center;padding:11px}.qr img{display:block;width:100%;height:100%;object-fit:contain}.placeholder{flex-direction:column;gap:18px;color:#9299a5;font-size:13px;text-align:center;padding:25px;background:#fafbfd}.scan-tip{text-align:center;color:#7b8391;font-size:13px;line-height:1.7;margin:15px 0}.order{text-align:center;color:#9a9fac;font-size:11px;margin:15px 0 23px;overflow-wrap:anywhere}.order span{display:block;margin-bottom:5px}code{font-size:11px}.result{text-align:center;padding-top:15px}.result h2{margin:20px 0 15px}.result p{color:#7a8392;font-size:14px;line-height:1.8}.result code{display:block;margin:22px 0 30px;color:#8c93a0;overflow-wrap:anywhere}.success-icon{display:grid;place-items:center;margin:0 auto;width:72px;height:72px;border-radius:50%;background:#eaf9f0;color:#22a667;font-size:39px}.spinner{width:28px;height:28px;border:3px solid #e1e8f5;border-top-color:#3d7fec;border-radius:50%;animation:spin .8s linear infinite}@keyframes spin{to{transform:rotate(360deg)}}@media(max-width:540px){.checkout-backdrop{padding:16px}.checkout-dialog{padding:28px 22px;border-radius:20px;max-height:calc(100dvh - 32px)}h2{font-size:24px}.close{right:16px;top:16px}label{padding:13px 10px;gap:7px;font-size:13px}.qr{width:256px;height:256px}}
</style>
