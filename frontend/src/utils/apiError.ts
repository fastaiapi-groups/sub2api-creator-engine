export function extractApiErrorMessage(e:any,fallback='请求失败'){return String(e?.message||e?.response?.data?.message||fallback)}
