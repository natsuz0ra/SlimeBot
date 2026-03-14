import { createRouter, createWebHistory } from 'vue-router'

import HomePage from '@/pages/HomePage.vue'
import LoginPage from '@/pages/LoginPage.vue'
import SettingsPage from '@/pages/SettingsPage.vue'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'login', component: LoginPage, meta: { requiresAuth: false } },
    { path: '/', name: 'home', component: HomePage, meta: { requiresAuth: true } },
    { path: '/chat/new_chat', name: 'new-chat', component: HomePage, meta: { requiresAuth: true } },
    { path: '/chat/:sessionId', name: 'chat', component: HomePage, meta: { requiresAuth: true } },
    { path: '/settings', name: 'settings', component: SettingsPage, meta: { requiresAuth: true } },
  ],
})

router.beforeEach((to) => {
  const authStore = useAuthStore()
  if (!authStore.initialized) {
    authStore.hydrate()
  }

  const requiresAuth = to.meta.requiresAuth !== false
  if (requiresAuth && !authStore.isAuthenticated) {
    return '/login'
  }
  if (to.path === '/login' && authStore.isAuthenticated) {
    return '/'
  }
  return true
})

router.afterEach(() => {
  document.title = 'SlimeBot'
})

export default router
