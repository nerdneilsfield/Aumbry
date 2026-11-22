// Offline single-file demo entry (built with vite.config.poc.ts). No API, no URL routing: the router only exists
// because renderer components use <RouterLink>/useRoute; it runs on memory history so it never touches the address bar.
import { createApp } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { allowInlineDataImages } from '@/lib/url'
import PocApp from './PocApp.vue'
import '@/style.css'

allowInlineDataImages()

const router = createRouter({
  history: createMemoryHistory(),
  routes: [{ path: '/:rest(.*)*', component: { render: () => null } }],
})

createApp(PocApp).use(router).mount('#app')
