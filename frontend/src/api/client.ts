import axios from 'axios'
import { errorMessage } from './errors'
export const api=axios.create({baseURL:import.meta.env.VITE_API_BASE_URL||'/api/v1',timeout:120000})
api.interceptors.request.use(c=>{const token=localStorage.getItem('exam408_token');if(token)c.headers.Authorization=`Bearer ${token}`;return c})
api.interceptors.response.use(r=>r,err=>{if(err.response?.status===401){localStorage.removeItem('exam408_token');if(location.pathname!='/login')location.href='/login'}return Promise.reject(err)})
export function messageOf(err:unknown){if(axios.isAxiosError(err))return errorMessage(err.response?.data?.error||err.message,err.response?.status);return errorMessage(err instanceof Error?err.message:undefined)}

