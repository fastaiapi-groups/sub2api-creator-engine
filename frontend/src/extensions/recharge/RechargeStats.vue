<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import * as echarts from 'echarts'
import { apiClient } from '../../api/client'

interface DailyRecharge { date: string; alipay: number; wxpay: number; total: number }
interface DailyUsage { date: string; image_count: number; total_amount: number; token_amount: number }
interface SourceStats { usage: DailyUsage[]; request_count: number; image_count: number; total_amount: number }
interface Stats { daily: DailyRecharge[]; yesterday_total: number; today_total: number; usage: DailyUsage[]; new_api: SourceStats; sub_api: SourceStats; new_api_daily: DailyRecharge[]; new_api_yesterday_total: number; new_api_today_total: number }
const emptySource = (): SourceStats => ({ usage: [], request_count: 0, image_count: 0, total_amount: 0 })
const stats = ref<Stats>({ daily: [], yesterday_total: 0, today_total: 0, usage: [], new_api: emptySource(), sub_api: emptySource(), new_api_daily: [], new_api_yesterday_total: 0, new_api_today_total: 0 })
const source = ref<'sub_api' | 'new_api'>('sub_api')
const activeSource = computed(() => stats.value[source.value])
const activeRecharge = computed(() => source.value === 'sub_api' ? stats.value.daily : stats.value.new_api_daily)
const activeYesterday = computed(() => source.value === 'sub_api' ? stats.value.yesterday_total : stats.value.new_api_yesterday_total)
const activeToday = computed(() => source.value === 'sub_api' ? stats.value.today_total : stats.value.new_api_today_total)
const error = ref(''), loading = ref(true)
const rechargeChart = ref<HTMLElement | null>(null), usageChart = ref<HTMLElement | null>(null), amountChart = ref<HTMLElement | null>(null)
let rechargeInstance: echarts.ECharts | undefined, usageInstance: echarts.ECharts | undefined, amountInstance: echarts.ECharts | undefined
function chartData(rows: DailyRecharge[], field: 'alipay' | 'wxpay' | 'total') { return rows.map(row => row[field]) }
function render() {
  if (rechargeChart.value) {
    rechargeInstance?.dispose(); rechargeInstance = echarts.init(rechargeChart.value)
    rechargeInstance.setOption({ tooltip: { trigger: 'axis', valueFormatter: (value: number) => `¥${value.toFixed(2)}` }, legend: { data: ['支付宝', '微信支付'] }, grid: { left: 55, right: 20, top: 42, bottom: 28 }, xAxis: { type: 'category', data: activeRecharge.value.map(x => x.date.slice(5)) }, yAxis: { type: 'value', name: 'RMB' }, series: [{ name: '支付宝', type: 'line', smooth: true, data: chartData(activeRecharge.value, 'alipay'), itemStyle: { color: '#168fe1' } }, { name: '微信支付', type: 'line', smooth: true, data: chartData(activeRecharge.value, 'wxpay'), itemStyle: { color: '#1aae68' } }] })
  }
  if (usageChart.value) {
    usageInstance?.dispose(); usageInstance = echarts.init(usageChart.value)
    usageInstance.setOption({ tooltip: { trigger: 'axis', valueFormatter: (value: number) => `${Number(value).toLocaleString('zh-CN')} 张` }, legend: { data: ['图片总数'] }, grid: { left: 70, right: 20, top: 42, bottom: 28 }, xAxis: { type: 'category', data: activeSource.value.usage.map(x => x.date.slice(5)) }, yAxis: [{ type: 'value', name: '图片数量' }], series: [{ name: '图片总数', type: 'bar', data: activeSource.value.usage.map(x => x.image_count), itemStyle: { color: '#8b5cf6' } }] })
  }
  if (amountChart.value) {
    amountInstance?.dispose(); amountInstance = echarts.init(amountChart.value)
    amountInstance.setOption({ tooltip: { trigger: 'axis', valueFormatter: (value: number) => `¥${Number(value).toFixed(2)}` }, legend: { data: ['图片总金额(RMB)', 'Token金额(RMB)'] }, grid: { left: 70, right: 20, top: 42, bottom: 28 }, xAxis: { type: 'category', data: activeSource.value.usage.map(x => x.date.slice(5)) }, yAxis: [{ type: 'value', name: 'RMB' }], series: [{ name: '图片总金额(RMB)', type: 'line', smooth: true, data: activeSource.value.usage.map(x => x.total_amount), itemStyle: { color: '#ef7d35' } }, { name: 'Token金额(RMB)', type: 'line', smooth: true, data: activeSource.value.usage.map(x => x.token_amount), itemStyle: { color: '#e74c8b' } }] })
  }
}
function fillDates<T extends { date: string }>(rows: T[], factory: (date: string) => T) {
  const byDate = new Map(rows.map(row => [row.date, row])); const out: T[] = []
  const parts = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(new Date()); const today = new Date(Date.UTC(Number(parts.find(x => x.type === 'year')?.value), Number(parts.find(x => x.type === 'month')?.value) - 1, Number(parts.find(x => x.type === 'day')?.value)))
  for (let i = 29; i >= 0; i--) { const d = new Date(today); d.setUTCDate(d.getUTCDate() - i); const date = d.toISOString().slice(0, 10); out.push(byDate.get(date) || factory(date)) }
  return out
}
function resize() { rechargeInstance?.resize(); usageInstance?.resize(); amountInstance?.resize() }
async function load() { loading.value = true; error.value = ''; try { const data = (await apiClient.get('/recharge/admin/stats')).data as Stats; const newAPI = data.new_api || { ...emptySource(), usage: data.usage || [] }; const subAPI = data.sub_api || emptySource(); stats.value = { ...data, new_api_daily: fillDates(data.new_api_daily || [], date => ({ date, alipay: 0, wxpay: 0, total: 0 })), new_api_yesterday_total: data.new_api_yesterday_total || 0, new_api_today_total: data.new_api_today_total || 0, daily: fillDates(data.daily, date => ({ date, alipay: 0, wxpay: 0, total: 0 })), usage: fillDates(data.usage || [], date => ({ date, image_count: 0, total_amount: 0, token_amount: 0 })), new_api: { ...newAPI, usage: fillDates(newAPI.usage || [], date => ({ date, image_count: 0, total_amount: 0, token_amount: 0 })) }, sub_api: { ...subAPI, usage: fillDates(subAPI.usage || [], date => ({ date, image_count: 0, total_amount: 0, token_amount: 0 })) } }; render() } catch (e: any) { error.value = e?.response?.data?.message || e?.message || '统计数据加载失败' } finally { loading.value = false } }
onMounted(() => { load(); window.addEventListener('resize', resize) })
watch(source, render)
onUnmounted(() => { window.removeEventListener('resize', resize); rechargeInstance?.dispose(); usageInstance?.dispose(); amountInstance?.dispose() })
</script>
<template>
  <section class="stats-panel">
    <div class="stats-heading"><div><h2>充值与用量统计</h2><p>管理员可查看最近 30 天的支付和调用数据。</p></div><button :disabled="loading" @click="load">刷新数据 ↻</button></div>
    <div v-if="error" class="message error" role="alert">{{ error }}</div>
    <div class="source-tabs"><button :class="{ active: source === 'sub_api' }" @click="source = 'sub_api'">sub-api 使用统计</button><button :class="{ active: source === 'new_api' }" @click="source = 'new_api'">new-api 使用统计</button></div>
    <div class="source-recharge-label">{{ source === 'sub_api' ? 'sub-api 充值数据' : 'new-api 充值数据' }}</div><div class="source-summary"><div><span>成功请求数</span><strong>{{ activeSource.request_count.toLocaleString('zh-CN') }}</strong></div><div><span>图片总数</span><strong>{{ activeSource.image_count.toLocaleString('zh-CN') }}</strong></div><div><span>图片总金额</span><strong>¥{{ activeSource.total_amount.toFixed(2) }}</strong></div></div>
    <div class="summary"><div><span>昨日收款总额</span><strong>¥{{ activeYesterday.toFixed(2) }}</strong></div><div><span>今日收款总额</span><strong>¥{{ activeToday.toFixed(2) }}</strong></div></div>
    <div class="chart-card"><h3>近 30 天充值总额</h3><div ref="rechargeChart" class="chart"></div></div>
    <div class="chart-card"><h3>近 30 天图片数量</h3><p class="hint">按使用记录中的 Image count 汇总。</p><div ref="usageChart" class="chart"></div></div>
    <div class="chart-card"><h3>近 30 天金额</h3><p class="hint">图片金额和 Token 调用金额均按成功记录的实际费用字段汇总，按 1:1 换算为 RMB。</p><div ref="amountChart" class="chart"></div></div>
  </section>
</template>
<style scoped>
.stats-panel{background:#fff;border:1px solid #e3e9f2;border-radius:12px;padding:18px;color:#34435b}.stats-heading{display:flex;align-items:center;justify-content:space-between;gap:20px}.stats-heading h2{margin:0;font-size:22px}.stats-heading p{font-size:12px;color:#8b96a7;margin:5px 0 0}.stats-panel button{border:1px solid #dfe3ec;background:#fff;color:#353b49;border-radius:8px;padding:9px 13px;font-size:12px}.source-recharge-label{font-size:13px;font-weight:650;color:#34435b;margin:14px 0 8px}.source-tabs{display:flex;gap:8px;margin:16px 0 12px}.source-tabs button.active{background:#edf3ff;border-color:#9bbcfb;color:#2864e8;font-weight:650}.source-summary{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px}.source-summary>div{background:#f8fafc;border:1px solid #e4eaf2;border-radius:9px;padding:11px 14px;display:flex;flex-direction:column;gap:5px}.source-summary span{font-size:11px;color:#8995a7}.source-summary strong{font-size:20px;color:#34435b}.summary{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px;margin:20px 0 14px}.summary>div{background:#f7f9fd;border:1px solid #e3e9f2;border-radius:10px;padding:14px 16px;display:flex;flex-direction:column;gap:7px}.summary span{font-size:12px;color:#8995a7}.summary strong{font-size:25px;color:#2864e8}.chart-card{border:1px solid #e3e9f2;border-radius:10px;padding:14px;margin-top:14px}.chart-card h3{font-size:14px;margin:0 0 4px}.hint{font-size:11px;color:#8b96a7;margin:0}.chart{height:300px;margin-top:8px}.message{padding:12px;border-radius:8px;margin-top:14px}.error{background:#fff0f0;color:#ad3333}@media(max-width:600px){.stats-heading{align-items:flex-start}.source-summary{grid-template-columns:1fr}.summary{grid-template-columns:1fr}.chart{height:240px}}
</style>
