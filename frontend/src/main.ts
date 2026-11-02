import { createApp } from 'vue'
import App from './App.vue'
import './style.css'
// 工具箱共享样式：面板已拆成独立组件，scoped 样式无法跨组件生效，因此集中为一份全局样式表。
import './styles/toolkit.css'

createApp(App).mount('#app')
