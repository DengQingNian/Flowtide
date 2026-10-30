// 管理台入口：安装路由、主题与全局异常兜底。
import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import { initTheme } from './lib/theme'
import './styles/main.css'

initTheme()

const app = createApp(App)
app.use(router)
app.config.errorHandler = (error, _instance, info) => {
  console.error('[Flowtide 管理台] 未捕获异常', error, info)
}
if (typeof window !== 'undefined') {
  window.addEventListener('unhandledrejection', (event) => {
    console.error('[Flowtide 管理台] 未处理的 Promise 异常', event.reason)
  })
}
app.mount('#app')
