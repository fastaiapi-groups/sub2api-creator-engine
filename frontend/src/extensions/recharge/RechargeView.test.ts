import { beforeEach, afterEach, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import RechargeView from './RechargeView.vue'
import RechargeOrders from './RechargeOrders.vue'
import { apiClient } from '../../api/client'
vi.mock('../../api/client',()=>({apiClient:{get:vi.fn(),post:vi.fn()}}))
const partial={id:'RA-partial-payment-order',package_id:'one',amount_cents:100,credit:'1',paid_amount_cents:1,credited_credit:'0.01000000',payment_type:'wxpay',status:'paid',created_at:'2026-10-18T12:00:00Z',paid_at:'2026-10-18T12:01:00Z',reversed_at:null,gateway_status:'paid'}
let wrapper:ReturnType<typeof mount>
let admin=false
beforeEach(()=>{vi.clearAllMocks();vi.useFakeTimers();admin=false;localStorage.setItem('auth_token','test-user');vi.mocked(apiClient.get).mockImplementation(async(path:any)=>({data:path==='/proxy/me'?{email:'demo@example.com',balance:'0.01',role:'admin'}:path==='/recharge/access'?{is_admin:admin}:path==='/recharge/packages'?[{id:'one',name:'1 刀额度',amount_cents:100,credit:'1'}]:path==='/recharge/logs'?[{order_id:partial.id,event:'credited',created_at:partial.paid_at,detail:JSON.stringify({credit:'0.01',paid_amount_cents:1,balance_after:'0.01'})}]:path.endsWith('/issues')?[]:[partial]}))})
afterEach(()=>{wrapper?.unmount();vi.useRealTimers();localStorage.clear()})
it('shows ordinary users only their overview, actual paid amount and own logs; ignores a claimed admin role',async()=>{
 wrapper=mount(RechargeView);await flushPromises()
 expect(wrapper.text()).toContain('我的充值记录');expect(wrapper.text()).toContain('我的到账日志')
 expect(wrapper.text()).toContain('实付 ¥0.01');expect(wrapper.text()).toContain('到账 $0.01')
 expect(wrapper.findAll('nav button').some(b=>b.text().includes('充值管理'))).toBe(false)
 expect(vi.mocked(apiClient.get).mock.calls.some(c=>String(c[0]).includes('/admin/'))).toBe(false)
})
it('opens admin management on all orders and clearly explains an empty exception queue',async()=>{
 admin=true;wrapper=mount(RechargeView);await flushPromises()
 await wrapper.findAll('nav button').find(b=>b.text().includes('充值管理'))!.trigger('click');await flushPromises()
 expect(wrapper.text()).toContain('应付 / 实付 / 到账');expect(wrapper.text()).toContain('实付 ¥0.01')
 await wrapper.findAll('button').find(b=>b.text().includes('异常订单'))!.trigger('click')
 expect(wrapper.text()).toContain('没有待处理异常。已付款订单会自动入账')
})
it('paginates orders without hiding later records or claiming quoted credit was received',async()=>{
 wrapper=mount(RechargeOrders,{props:{orders:[partial,{...partial,id:'RA-second-payment'}],busy:false,pageSize:1}})
 expect(wrapper.find('code').attributes('title')).toBe(partial.id)
 expect(wrapper.text()).toContain('$0.01');expect(wrapper.text()).not.toContain('$1 ')
 await wrapper.findAll('button').find(b=>b.text()==='下一页')!.trigger('click')
 expect(wrapper.find('code').attributes('title')).toBe('RA-second-payment')
})
