import axios from 'axios'
import { errorMessage } from './errors'
export const api=axios.create({baseURL:import.meta.env.VITE_API_BASE_URL||'/api/v1',timeout:120000})
api.interceptors.request.use(c=>{const token=localStorage.getItem('exam408_token');if(token)c.headers.Authorization=`Bearer ${token}`;return c})
api.interceptors.response.use(r=>r,err=>{if(err.response?.status===401){localStorage.removeItem('exam408_token');if(location.pathname!='/login')location.href='/login'}return Promise.reject(err)})
export function messageOf(err:unknown){if(axios.isAxiosError(err))return errorMessage(err.response?.data?.error||err.message,err.response?.status);return errorMessage(err instanceof Error?err.message:undefined)}


const pageReads=new Map<string,{expires:number;request:Promise<unknown>}>()
export function clearPageReads(){pageReads.clear()}
export function readPage<T>(url:string,params?:Record<string,unknown>):Promise<T>{
 const key=JSON.stringify([localStorage.getItem('exam408_token'),url,params])
 const current=pageReads.get(key)
 if(current&&current.expires>Date.now())return current.request as Promise<T>
 if(pageReads.size>=48)pageReads.delete(pageReads.keys().next().value!)
 const request=api.get<T>(url,{params}).then(({data})=>data)
 const entry={expires:Date.now()+45000,request}
 pageReads.set(key,entry)
 void request.catch(()=>{if(pageReads.get(key)===entry)pageReads.delete(key)})
 return request
}
api.interceptors.response.use(response=>{
 const method=response.config.method
 if(method&&method!=='get'&&method!=='head'&&response.config.url!=='/exam408/learning-events')clearPageReads()
 return response
})
