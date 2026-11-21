import { createRouter, createWebHistory } from 'vue-router'

// Views are lazy so the heavy reader dependencies (shiki, katex) load only when needed.
const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'home', component: () => import('../views/HomeView.vue') },
    { path: '/d/:id', name: 'directory', component: () => import('../views/DirectoryView.vue') },
    { path: '/r/:id', name: 'report', component: () => import('../views/ReaderView.vue') },
    { path: '/p/:id', name: 'post', component: () => import('../views/ReaderView.vue') },
    { path: '/admin', name: 'admin', component: () => import('../views/AdminView.vue') },
    { path: '/:rest(.*)*', name: 'not-found', component: () => import('../views/NotFoundView.vue') },
  ],
  scrollBehavior: (to, from) => (to.path === from.path ? undefined : { top: 0 }),
})

export default router
