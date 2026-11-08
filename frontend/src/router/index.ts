import {createRouter,createWebHistory} from 'vue-router'
import {useAuthStore} from '@/stores/auth'
import {api} from '@/api/client'
import AppLayout from '@/components/AppLayout.vue'
const router=createRouter({history:createWebHistory(),routes:[{path:'/login',component:()=>import('@/views/LoginView.vue'),meta:{public:true}},{path:'/',component:AppLayout,redirect:'/exam408',children:[{path:'exam408/admin',component:()=>import('@/views/AdminDashboardView.vue'),meta:{admin:true}},{path:'exam408/admin/users',component:()=>import('@/views/AdminUsersView.vue'),meta:{admin:true}},{path:'exam408/admin/resources',component:()=>import('@/views/ResourcesView.vue'),meta:{admin:true}},{path:'exam408/admin/knowledge',component:()=>import('@/views/KnowledgeView.vue'),meta:{admin:true}},{path:'exam408/admin/assets',component:()=>import('@/views/AssetsView.vue'),meta:{admin:true}},{path:'exam408/admin/graph',component:()=>import('@/views/GraphView.vue'),meta:{admin:true}},{path:'exam408',component:()=>import('@/views/HomeView.vue')},{path:'exam408/learning',redirect:to=>({path:'/exam408/tasks',query:{...to.query,tab:'teacher'}})},{path:'exam408/knowledge',component:()=>import('@/views/KnowledgeHubView.vue')},{path:'exam408/graph',redirect:to=>({path:'/exam408/knowledge',query:{...to.query,tab:'graph'}})},{path:'exam408/resources',component:()=>import('@/views/ResourcesView.vue')},{path:'exam408/assistant',component:()=>import('@/views/AssistantView.vue')},{path:'exam408/weakness',component:()=>import('@/views/WeaknessView.vue')},{path:'exam408/statistics',component:()=>import('@/views/DashboardView.vue')},{path:'exam408/assets',component:()=>import('@/views/AssetsView.vue')},{path:'exam408/assessment',component:()=>import('@/views/AssessmentView.vue')},{path:'exam408/tasks',component:()=>import('@/views/TasksHubView.vue')},{path:'exam408/settings',component:()=>import('@/views/SettingsView.vue')}]},{path:'/:pathMatch(.*)*',redirect:'/exam408'}]})
router.beforeEach(async to=>{
 const auth=useAuthStore(),signed=!!localStorage.getItem('exam408_token')
 if(!to.meta.public&&!signed)return '/login'
 if(signed&&!auth.user){try{await auth.load()}catch{auth.logout();return '/login'}}
 if(to.meta.admin&&auth.user?.role!=='admin')return '/exam408'
 if(to.path==='/exam408/assessment'&&['weakness','statistics'].includes(String(to.query.tab))){const {tab,...query}=to.query;return {path:tab==='weakness'?'/exam408/weakness':'/exam408/statistics',query,hash:to.hash,replace:true}}
 if((to.path==='/exam408'||to.path==='/login')&&auth.user?.role==='admin')return '/exam408/admin'
 if(to.path==='/login'&&signed)return '/exam408'
 if(to.path==='/exam408'&&auth.user?.role!=='admin'){try{const {data}=await api.get('/exam408/learning/overview');if(data.NeedsPretest)return '/exam408/assessment'}catch{/* keep dashboard available when optional overview fails */}}
})
export default router
