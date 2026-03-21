import { createApp } from 'vue'
import './styles/index.css'
import 'highlight.js/styles/github-dark.css'
import App from './App.vue'

import { createPinia } from 'pinia'
import router from './router'
import { i18n } from './i18n'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(i18n)
app.mount('#app')
