import {apiClient} from './client'
export const userGroupsAPI={getAvailable:async()=> (await apiClient.get('/proxy/groups/available')).data}
export const keysAPI={
 list:async(page=1,pageSize=100,filters:any={})=>(await apiClient.get('/proxy/keys',{params:{page,page_size:pageSize,...filters}})).data,
 create:async(name:string,group_id?:number|null)=>(await apiClient.post('/proxy/keys',{name,group_id})).data,
 update:async(id:number,updates:any)=>(await apiClient.put(`/proxy/keys/${id}`,updates)).data
}
