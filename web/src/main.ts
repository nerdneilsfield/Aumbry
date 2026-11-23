import { createApp } from 'vue'
import { MutationCache, QueryCache, QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import App from './App.vue'
import router from './router'
import { ApiError } from './api/client'
import { clearAuthSensitiveQueries } from './lib/queryCache'
import './style.css'

// An expired admin session (401 on any admin call except login/me) drops the UI back to the login form
// and evicts any privileged item/revision caches from the SPA session.
const sessionExpired = (e: unknown, key?: readonly unknown[]) => {
  if (e instanceof ApiError && e.status === 401 && key?.[0] === 'admin' && key[1] !== 'login' && key[1] !== 'me') {
    clearAuthSensitiveQueries(queryClient)
    queryClient.setQueryData(['admin', 'me'], { admin: false })
  }
}

const queryClient: QueryClient = new QueryClient({
  defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false, staleTime: 30_000 } },
  queryCache: new QueryCache({ onError: (e, q) => sessionExpired(e, q.queryKey) }),
  mutationCache: new MutationCache({ onError: (e, _v, _c, m) => sessionExpired(e, m.options.mutationKey) }),
})

createApp(App).use(VueQueryPlugin, { queryClient }).use(router).mount('#app')
