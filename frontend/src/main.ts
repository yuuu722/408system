import { createApp } from 'vue'
import { createPinia } from 'pinia'
import 'tdesign-vue-next/es/style/index.css'
import App from './App.vue'
import router from './router'
import './styles.css'
createApp(App).use(createPinia()).use(router).mount('#app')
