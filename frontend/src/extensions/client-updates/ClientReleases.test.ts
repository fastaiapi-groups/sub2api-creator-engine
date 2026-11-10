import {beforeEach,afterEach,it,expect,vi} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import ClientReleases from './ClientReleases.vue'
import {apiClient} from '../../api/client'
import androidRelease from './android-release.json'
vi.mock('../../api/client',()=>({apiClient:{get:vi.fn(),post:vi.fn()}}))
let wrapper:ReturnType<typeof mount>|undefined
let release:any
beforeEach(()=>{vi.clearAllMocks();history.replaceState(null,'','/fastaiui/admin/client-releases?product_id=xiaoxiao-manju');release={id:12,product_id:'xiaoxiao-manju',version:'0.2.12',status:'draft',notes:'已保存说明',assets:[{platform:'renderer',arch:'any',size_bytes:1234}],required_targets:['renderer/any'],options:{mandatory:false}};vi.mocked(apiClient.get).mockImplementation(async(url)=>({data:String(url).includes('/public')?{release:null}:String(url).includes('/access')?{is_admin:true}:[release]}) as any)})
afterEach(()=>wrapper?.unmount())
it('shows the verified English Android APK beside the signed desktop release without changing desktop metadata',async()=>{
 history.replaceState(null,'','/fastaiui/downloads/manju')
 const desktop={...release,status:'published',assets:[{platform:'macos',arch:'arm64',name:'Manju.dmg',url:'https://downloads.example/Manju.dmg',size_bytes:2048,sha256:'c'.repeat(64)}]}
 vi.mocked(apiClient.get).mockResolvedValue({data:{release:desktop,installer_release:desktop}} as any)
 wrapper=mount(ClientReleases);await flushPromises()
 const card=wrapper.find('.md-android-card')
 expect(card.text()).toContain('Android 8.0')
 expect(card.find('.md-download-button').attributes('href')).toBe(androidRelease.url)
 expect(androidRelease.filename).toMatch(/^[A-Za-z0-9.-]+\.apk$/)
 expect(androidRelease.sha256).toMatch(/^[a-f0-9]{64}$/)
 expect(androidRelease.size_bytes).toBeGreaterThan(0)
 expect(card.text()).toContain(androidRelease.sha256)
 expect(card.find('.md-android-qr a').attributes('href')).toBe(androidRelease.url)
 const qr=card.find('.md-android-qr img')
 expect(qr.attributes('alt')).toContain('扫描二维码下载 Android')
 const svg=decodeURIComponent(qr.attributes('src').split(',').slice(1).join(','))
 expect(svg).toContain('<svg')
 expect(svg).toContain('#193b2d')
 expect(card.text()).toContain('手机扫码下载')
 expect(wrapper.find('.md-download-button').attributes('href')).toBe(desktop.assets[0].url)
 expect(apiClient.post).not.toHaveBeenCalled()
})
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
it('renders signed notes as separate readable entries and keeps the last full installer when a newer renderer is published',async()=>{
 history.replaceState(null,'','/fastaiui/downloads/manju')
 const installer={...release,version:'0.2.19',status:'published',assets:[{platform:'macos',arch:'arm64',name:'Manju.dmg',url:'https://fastaiapi.cloud/fastaiapi/manju-updates/downloads/manju/0.2.19/abc/Manju.dmg',size_bytes:2048,sha256:'a'.repeat(64)}]}
 const current={...release,version:'0.2.20',status:'published',notes:'保留账号与工程。优化任务提醒。\r\n支持 0.2.19 及以后版本\n下载说明 https://example.com/v0.2.19'}
 vi.mocked(apiClient.get).mockResolvedValue({data:{release:current,installer_release:installer}} as any)
 wrapper=mount(ClientReleases);await flushPromises()
 expect(wrapper.findAll('.md-release-list li').map(li=>li.text())).toEqual(['保留账号与工程。','优化任务提醒。','支持 0.2.19 及以后版本','下载说明 https://example.com/v0.2.19'])
 expect(current.notes).toBe('保留账号与工程。优化任务提醒。\r\n支持 0.2.19 及以后版本\n下载说明 https://example.com/v0.2.19')
 expect(wrapper.find('.md-release-heading').text()).toContain('v0.2.20')
 expect(wrapper.find('.md-card-version').text()).toBe('v0.2.19')
 expect(wrapper.find('.md-download-button').attributes('href')).toBe(installer.assets[0].url)
 expect(wrapper.find('.md-card-bottom a').attributes('href')).toBe('http://8.137.194.180:90/hub/updates/downloads/manju/0.2.19/abc/Manju.dmg')
 expect(wrapper.find('form').exists()).toBe(false)
 expect(apiClient.get).toHaveBeenCalledTimes(1)
 expect(apiClient.post).not.toHaveBeenCalled()
})
it('keeps note content as plain text and copies the exact file checksum',async()=>{
 history.replaceState(null,'','/fastaiui/downloads/manju')
 const checksum='b'.repeat(64)
 vi.mocked(apiClient.get).mockResolvedValue({data:{release:{...release,status:'published',notes:'<img src=x onerror=alert(1)>\n使用新版。',assets:[{platform:'windows',arch:'x64',name:'Manju.exe',url:'https://downloads.example/Manju.exe',size_bytes:1024,sha256:checksum}]}}} as any)
 const writeText=vi.fn().mockResolvedValue(undefined)
 Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText}})
 wrapper=mount(ClientReleases);await flushPromises()
 expect(wrapper.find('.md-release-list img').exists()).toBe(false)
 expect(wrapper.find('.md-release-list').text()).toContain('<img src=x onerror=alert(1)>')
 expect(wrapper.find('.md-card-bottom a').exists()).toBe(false)
 await wrapper.find('.md-file-check button').trigger('click');await flushPromises()
 expect(writeText).toHaveBeenCalledWith(checksum)
 expect(wrapper.find('[role=status]').text()).toContain('SHA-256 已复制')
 await wrapper.find('button[aria-label="关闭提示"]').trigger('click')
 expect(wrapper.find('[role=status]').exists()).toBe(false)
})
