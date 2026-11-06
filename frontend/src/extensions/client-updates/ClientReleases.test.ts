import {beforeEach,afterEach,it,expect,vi} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import ClientReleases from './ClientReleases.vue'
import {apiClient} from '../../api/client'
vi.mock('../../api/client',()=>({apiClient:{get:vi.fn(),post:vi.fn()}}))
let wrapper:ReturnType<typeof mount>|undefined
let release:any
beforeEach(()=>{vi.clearAllMocks();history.replaceState(null,'','/fastaiui/admin/client-releases?product_id=xiaoxiao-manju');release={id:12,product_id:'xiaoxiao-manju',version:'0.2.12',status:'draft',notes:'已保存说明',assets:[{platform:'renderer',arch:'any',size_bytes:1234}],required_targets:['renderer/any'],options:{mandatory:false}};vi.mocked(apiClient.get).mockImplementation(async(url)=>({data:String(url).includes('/public')?{release:null}:String(url).includes('/access')?{is_admin:true}:[release]}) as any)})
afterEach(()=>wrapper?.unmount())
it('saves current notes and required policy before signing and only calls the isolated release API',async()=>{
 wrapper=mount(ClientReleases);await flushPromises();await wrapper.find('.release-list button').trigger('click');await flushPromises()
 await wrapper.find('.editor textarea').setValue('最新更新说明');await wrapper.find('.editor input[type=checkbox]').setValue(true)
 vi.mocked(apiClient.post).mockImplementation(async(url,body:any)=>{if(String(url).endsWith('/options?product_id=xiaoxiao-manju'))release={...release,notes:body.notes,options:{mandatory:body.mandatory}};else if(String(url).includes('/prepare?'))release={...release,status:'ready'};return {data:release} as any})
 await wrapper.findAll('button').find(b=>b.text()==='校验并签名')!.trigger('click');await flushPromises()
 expect(apiClient.post).toHaveBeenNthCalledWith(1,'/manju-updates/api/client-updates/admin/releases/12/options?product_id=xiaoxiao-manju',{notes:'最新更新说明',mandatory:true})
 expect(apiClient.post).toHaveBeenNthCalledWith(2,'/manju-updates/api/client-updates/admin/releases/12/prepare?product_id=xiaoxiao-manju')
 expect(wrapper.text()).toContain('发布到官网与客户端')
 for(const [url] of vi.mocked(apiClient.get).mock.calls)expect(String(url)).toMatch(/^\/manju-updates\/api\/client-updates\//)
})
it('hides publishing controls from a normal user and keeps the original Codex product endpoint',async()=>{
 history.replaceState(null,'','/fastaiui/admin/client-releases');vi.mocked(apiClient.get).mockImplementation(async(url)=>({data:String(url).includes('public')?{release:null}:{is_admin:false}}) as any)
 wrapper=mount(ClientReleases);await flushPromises();expect(wrapper.text()).toContain('需要管理员权限');expect(wrapper.find('form').exists()).toBe(false);expect(apiClient.post).not.toHaveBeenCalled();expect(apiClient.get).toHaveBeenCalledWith('/client-updates/public?product_id=fastaiapi-codexplus')
})
