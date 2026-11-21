<script setup lang="ts">
import { computed, ref } from 'vue'
import { useMutation } from '@tanstack/vue-query'
import { Lock } from 'lucide-vue-next'
import { ApiError, api } from '@/api/client'
import StatePage from './StatePage.vue'

const props = defineProps<{ dirId: string; dirName?: string }>()
const emit = defineEmits<{ unlocked: [] }>()

const password = ref('')
const unlock = useMutation({
  mutationFn: () => api.unlock(props.dirId, password.value),
  onSuccess: () => emit('unlocked'),
})

const error = computed(() => {
  const e = unlock.error.value
  if (!e) return ''
  if (e instanceof ApiError && e.status === 401) return '密码错误'
  if (e instanceof ApiError && e.status === 429) return '尝试次数过多，请稍后再试'
  return '解锁失败，请稍后重试'
})
</script>

<template>
  <StatePage :icon="Lock" :title="dirName ? `「${dirName}」需要密码` : '该目录需要密码'" message="请输入访问密码以查看目录内容。">
    <form class="flex w-full max-w-xs flex-col gap-3" @submit.prevent="unlock.mutate()">
      <input v-model="password" type="password" class="input" placeholder="访问密码" autocomplete="off" autofocus required />
      <p v-if="error" class="text-sm text-red-400">{{ error }}</p>
      <button class="btn btn-primary" :disabled="unlock.isPending.value || !password">解锁</button>
    </form>
  </StatePage>
</template>
