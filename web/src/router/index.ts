import { createRouter, createWebHistory } from 'vue-router'
import HomeView from '../views/HomeView.vue'
import DirectoryView from '../views/DirectoryView.vue'
import ReaderView from '../views/ReaderView.vue'
import AdminView from '../views/AdminView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'home', component: HomeView },
    { path: '/d/:id', name: 'directory', component: DirectoryView },
    { path: '/r/:id', name: 'report', component: ReaderView },
    { path: '/p/:id', name: 'post', component: ReaderView },
    { path: '/admin', name: 'admin', component: AdminView },
  ],
})

export default router
