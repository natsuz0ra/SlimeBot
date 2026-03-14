import { createRouter, createWebHistory } from 'vue-router'

import HomePage from '@/pages/HomePage.vue'
import SettingsPage from '@/pages/SettingsPage.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'home', component: HomePage },
    { path: '/chat/new_chat', name: 'new-chat', component: HomePage },
    { path: '/chat/:sessionId', name: 'chat', component: HomePage },
    { path: '/settings', name: 'settings', component: SettingsPage },
  ],
})

router.afterEach(() => {
  document.title = 'SlimeBot'
})

export default router
