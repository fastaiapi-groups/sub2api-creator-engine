import axios from 'axios'
export const apiClient=axios.create({baseURL:'/fastaiapi/api',timeout:600000})
apiClient.interceptors.request.use(c=>{const t=localStorage.getItem('auth_token')||localStorage.getItem('token');if(t)c.headers.Authorization=t.startsWith('Bearer ')?t:`Bearer ${t}`;return c})
apiClient.interceptors.response.use(r=>{const p=r.data;if(p&&typeof p==='object'&&'code'in p){if(p.code===0)r.data=p.data;else return Promise.reject({status:r.status,code:p.code,message:p.message})}return r})
export function buildGatewayUrl(path:string){const m:Record<string,string>={'/v1/models':'/fastaiapi/api/proxy/models','/v1/chat/completions':'/fastaiapi/api/proxy/chat','/v1/images/generations':'/fastaiapi/api/proxy/images','/v1/images/edits':'/fastaiapi/api/proxy/image-edits'};return m[path]||path}
