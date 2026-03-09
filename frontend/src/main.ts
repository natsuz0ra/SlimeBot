import { createApp } from 'vue'
import './style.css'
import 'highlight.js/styles/github-dark.css'
import App from './App.vue'
import TDesign from 'tdesign-vue-next'
import 'tdesign-vue-next/es/style/index.css'

import { createPinia } from 'pinia'
import router from './router'
import { i18n } from './i18n'

const app = createApp(App)
app.use(TDesign)
app.use(createPinia())
app.use(router)
app.use(i18n)
app.mount('#app')
