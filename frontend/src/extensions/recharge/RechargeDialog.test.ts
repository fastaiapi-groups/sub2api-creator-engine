import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import RechargeDialog from './RechargeDialog.vue'
import { apiClient } from '../../api/client'
import QRCode from 'qrcode'
import type { Order } from './types'
vi.mock('../../api/client', () => ({ apiClient: { post: vi.fn(), get: vi.fn() } }))
vi.mock('qrcode', () => ({ default: { toDataURL: vi.fn().mockResolvedValue('data:image/png;base64,fixture') } }))
const product = { id: '5', name: '5 刀额度', amount_cents: 500, credit: '5' }
const order: Order = { id: 'RA-test-order', package_id: '5', amount_cents: 500, credit: '5', payment_type: 'wxpay', status: 'pending', created_at: '', paid_at: null, reversed_at: null, gateway_status: '' }
let wrapper: ReturnType<typeof mount>
const button = (text: string) => wrapper.findAll('button').find(b => b.text() === text)!
function open(existingOrder?: Order) { wrapper = mount(RechargeDialog, { props: { product, email: 'demo@example.com', existingOrder }, global: { stubs: { teleport: true } }, attachTo: document.body }); return wrapper }
beforeEach(() => { vi.clearAllMocks(); vi.useFakeTimers(); document.body.style.overflow = '' })
afterEach(() => { wrapper?.unmount(); vi.useRealTimers(); document.body.innerHTML = '' })
describe('recharge confirmation and QR checkout', () => {
  it('opens confirmation without creating an order; submits the selected channel and renders native QR locally', async () => {
    open(); await flushPromises()
    expect(apiClient.post).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('demo@example.com'); expect(wrapper.text()).toContain('¥5.00')
    await wrapper.find('input[value="wxpay"]').setValue(true)
    vi.mocked(apiClient.post).mockResolvedValueOnce({ data: { order } }).mockResolvedValueOnce({ data: { order, state: 'ready', qr_code: 'wxp://native-test', qr_kind: 'text' } })
    const url = window.location.href
    await button('提交订单').trigger('click'); await flushPromises()
    expect(apiClient.post).toHaveBeenNthCalledWith(1, '/recharge/orders', expect.objectContaining({ package_id: '5', payment_type: 'wxpay', request_id: expect.any(String) }), expect.any(Object))
    expect(QRCode.toDataURL).toHaveBeenCalledWith('wxp://native-test', expect.any(Object))
    expect(wrapper.find('img').attributes('src')).toBe('data:image/png;base64,fixture')
    expect(wrapper.find('iframe').exists()).toBe(false); expect(window.location.href).toBe(url)
  })
  it('keeps the same request id and selected channel after an ambiguous local submission', async () => {
    open(); vi.mocked(apiClient.post).mockRejectedValueOnce(new Error('timeout'))
    await button('提交订单').trigger('click'); await flushPromises()
    const request = vi.mocked(apiClient.post).mock.calls[0][1]
    expect(wrapper.find('input[value="wxpay"]').attributes('disabled')).toBeDefined()
    vi.mocked(apiClient.post).mockResolvedValueOnce({ data: { order: { ...order, payment_type: 'alipay' } } }).mockResolvedValueOnce({ data: { order, state: 'uncertain' } })
    await button('提交订单').trigger('click'); await flushPromises()
    expect(vi.mocked(apiClient.post).mock.calls[1][1]).toEqual(request)
  })
  it('retries QR retrieval using the known order; never creates a second order', async () => {
    open(); vi.mocked(apiClient.post).mockResolvedValueOnce({ data: { order } }).mockRejectedValueOnce(new Error('gateway timeout'))
    await button('提交订单').trigger('click'); await flushPromises()
    vi.mocked(apiClient.post).mockResolvedValueOnce({ data: { order, state: 'ready', qr_kind: 'image', qr_code: 'data:image/png;base64,channel' } })
    await button('重试获取二维码').trigger('click'); await flushPromises()
    expect(vi.mocked(apiClient.post).mock.calls.filter(c => c[0] === '/recharge/orders')).toHaveLength(1)
    expect(wrapper.find('img').attributes('src')).toBe('data:image/png;base64,channel')
  })
  it('polls until confirmed paid, emits once, then stops and cleans up on close', async () => {
    vi.mocked(apiClient.post).mockResolvedValue({ data: { order, state: 'ready', qr_kind: 'image', qr_code: 'data:image/png;base64,channel' } })
    open(order); await flushPromises()
    vi.mocked(apiClient.get).mockResolvedValue({ data: { order: { ...order, status: 'paid' }, state: 'paid' } })
    await vi.advanceTimersByTimeAsync(3000); await flushPromises()
    expect(wrapper.text()).toContain('充值成功'); expect(wrapper.emitted('paid')).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(15000); expect(apiClient.get).toHaveBeenCalledTimes(1)
    const signal = vi.mocked(apiClient.post).mock.calls[0][2]?.signal as AbortSignal
    wrapper.unmount(); expect(signal.aborted).toBe(true); expect(document.body.style.overflow).toBe('')
  })
  it('shows the actual received credit after partial payment instead of the quoted package', async () => {
    vi.mocked(apiClient.post).mockResolvedValue({data:{order:{...order,status:'paid',paid_amount_cents:1,credited_credit:'0.01000000'},state:'paid'}})
    open(order);await flushPromises()
    expect(wrapper.text()).toContain('实付 ¥0.01');expect(wrapper.text()).toContain('$0.01 USD')
    expect(wrapper.text()).not.toContain('$5 USD')
  })
  it('removes QR and stops polling for an expired order', async () => {
    vi.mocked(apiClient.post).mockResolvedValue({ data: { order: { ...order, gateway_status: 'expired' }, state: 'expired', qr_code: 'must-not-show' } })
    open(order); await flushPromises()
    expect(wrapper.text()).toContain('二维码已过期'); expect(wrapper.find('img').exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(6000); expect(apiClient.get).not.toHaveBeenCalled()
  })
})
