import { createRouter, createWebHistory } from 'vue-router'

import HomePage from '../../pages/home/HomePage.vue'
import SettingsPage from '../../pages/settings/SettingsPage.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'home', component: HomePage },
    { path: '/settings', name: 'settings', component: SettingsPage },
  ],
})

router.afterEach(() => {
  document.title = 'Corner'
})

export default router
