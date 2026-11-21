<script setup lang="ts">
import { computed, ref } from 'vue'
import { ShieldCheck } from 'lucide-vue-next'
import { ApiError } from '@/api/client'
import { useAdmin } from '@/composables/useAdmin'

const { login } = useAdmin()
const username = ref('')
const password = ref('')

const error = computed(() => {
  const e = login.error.value
  if (!e) return ''
  if (e instanceof ApiError && e.status === 401) return '用户名或密码错误'
  if (e instanceof ApiError && e.status === 429) return '尝试次数过多，请稍后再试'
  return '登录失败，请稍后重试'
})
</script>

<template>
  <form class="card mx-auto mt-10 w-full max-w-sm space-y-4 p-8" @submit.prevent="login.mutate({ username, password })">
    <div class="text-center">
      <ShieldCheck class="mx-auto size-8 text-blue-300" />
      <h1 class="mt-3 text-xl font-semibold text-gray-50">管理员登录</h1>
    </div>
    <div>
      <label class="label" for="u">用户名</label>
      <input id="u" v-model="username" class="input" autocomplete="username" required autofocus />
    </div>
    <div>
      <label class="label" for="p">密码</label>
      <input id="p" v-model="password" type="password" class="input" autocomplete="current-password" required />
    </div>
    <p v-if="error" class="text-sm text-red-400">{{ error }}</p>
    <button class="btn btn-primary w-full" :disabled="login.isPending.value">登录</button>
  </form>
</template>
