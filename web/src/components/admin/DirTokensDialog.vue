<script setup lang="ts">
import { computed, ref } from 'vue'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { useClipboard } from '@vueuse/core'
import { Check, Copy, KeyRound, Plus, RefreshCw, Rss, Trash2 } from 'lucide-vue-next'
import { api, errorMessage } from '@/api/client'
import type { DirTokenInfo } from '@/api/types'
import ConfirmDialog from '../ConfirmDialog.vue'
import Modal from '../Modal.vue'

const props = defineProps<{ dirId: string; dirName: string; isProtected: boolean }>()
const emit = defineEmits<{ close: [] }>()

const qc = useQueryClient()
const label = ref('')
const revealedToken = ref<{ label: string; token: string } | null>(null)
const copiedKey = ref('')
const confirmTarget = ref<{ kind: 'rotate' | 'delete'; token: DirTokenInfo } | null>(null)

const tokensQuery = useQuery({
  queryKey: ['admin', 'dir-tokens', () => props.dirId],
  queryFn: () => api.admin.listDirTokens(props.dirId),
})
const tokens = computed(() => tokensQuery.data.value?.tokens ?? [])

const { copy } = useClipboard({ legacy: true })
function copyText(key: string, text: string) {
  void copy(text).then(() => {
    copiedKey.value = key
    setTimeout(() => {
      if (copiedKey.value === key) copiedKey.value = ''
    }, 2000)
  })
}

const encId = computed(() => encodeURIComponent(props.dirId))
const publicRssUrl = computed(() => `${location.origin}/feeds/${encId.value}/rss.xml`)
const publicPodcastUrl = computed(() => `${location.origin}/feeds/${encId.value}/podcast.xml`)
const secretRssUrl = computed(() =>
  revealedToken.value ? `${location.origin}/feeds/${encId.value}/rss.xml?token=${encodeURIComponent(revealedToken.value.token)}` : '',
)
const secretPodcastUrl = computed(() =>
  revealedToken.value ? `${location.origin}/feeds/${encId.value}/podcast.xml?token=${encodeURIComponent(revealedToken.value.token)}` : '',
)

const createMut = useMutation({
  mutationFn: () => api.admin.createDirToken(props.dirId, label.value.trim()),
  onSuccess: async (res) => {
    label.value = ''
    revealedToken.value = { label: res.label, token: res.token }
    await qc.invalidateQueries({ queryKey: ['admin', 'dir-tokens', props.dirId] })
  },
})

const rotateMut = useMutation({
  mutationFn: (tok: DirTokenInfo) => api.admin.rotateDirToken(props.dirId, tok.id),
  onSuccess: (res) => {
    const lbl = confirmTarget.value?.token.label ?? ''
    revealedToken.value = { label: lbl, token: res.token }
    confirmTarget.value = null
  },
})

const deleteMut = useMutation({
  mutationFn: (tok: DirTokenInfo) => api.admin.deleteDirToken(props.dirId, tok.id),
  onSuccess: async () => {
    confirmTarget.value = null
    await qc.invalidateQueries({ queryKey: ['admin', 'dir-tokens', props.dirId] })
  },
})

const activeErr = computed(() => tokensQuery.error.value ?? createMut.error.value ?? rotateMut.error.value ?? deleteMut.error.value)
function fmtDate(iso: string) {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function handleConfirm() {
  if (!confirmTarget.value) return
  if (confirmTarget.value.kind === 'rotate') rotateMut.mutate(confirmTarget.value.token)
  else deleteMut.mutate(confirmTarget.value.token)
}
</script>

<template>
  <Modal :title="`订阅与只读 Token：${dirName}`" wide @close="emit('close')">
    <div class="space-y-5">
      <div v-if="!isProtected" class="rounded-lg border border-blue-500/30 bg-blue-500/10 p-3 text-sm text-blue-100">
        <p class="mb-2 flex items-center gap-1.5 font-medium"><Rss class="size-4 text-blue-300" />当前目录为公开目录，无需 Token 即可订阅：</p>
        <div class="flex flex-wrap gap-2">
          <button type="button" class="btn !py-1 text-xs" @click="copyText('pub-rss', publicRssUrl)">
            <component :is="copiedKey === 'pub-rss' ? Check : Copy" class="size-3.5" />
            {{ copiedKey === 'pub-rss' ? '已复制 RSS' : '复制公开 RSS 地址' }}
          </button>
          <button type="button" class="btn !py-1 text-xs" @click="copyText('pub-pod', publicPodcastUrl)">
            <component :is="copiedKey === 'pub-pod' ? Check : Copy" class="size-3.5" />
            {{ copiedKey === 'pub-pod' ? '已复制 Podcast' : '复制公开 Podcast 地址' }}
          </button>
        </div>
      </div>

      <p v-else class="text-xs text-gray-400">
        受保护目录的 RSS / Podcast 订阅须携带绑定本目录的 <code class="text-gray-200">ad_...</code> 只读 Token；子目录若设了独立密码，需要为子目录单独签发 Token（改访客密码不会吊销 Token）。
      </p>

      <div v-if="revealedToken" class="space-y-2.5 rounded-lg border border-emerald-500/40 bg-emerald-500/10 p-3.5 text-sm">
        <p class="font-medium text-emerald-200">
          新 Token 已生成<template v-if="revealedToken.label">（{{ revealedToken.label }}）</template>：明文仅展示这一次，请立即复制保存。
        </p>
        <code class="block overflow-x-auto rounded bg-gray-900 px-2.5 py-1.5 font-mono text-xs text-emerald-300">{{ revealedToken.token }}</code>
        <div class="flex flex-wrap gap-2">
          <button type="button" class="btn !py-1 text-xs" @click="copyText('sec-tok', revealedToken.token)">
            <component :is="copiedKey === 'sec-tok' ? Check : Copy" class="size-3.5" />
            {{ copiedKey === 'sec-tok' ? '已复制 Token' : '复制 Token' }}
          </button>
          <button type="button" class="btn !py-1 text-xs" @click="copyText('sec-rss', secretRssUrl)">
            <component :is="copiedKey === 'sec-rss' ? Check : Copy" class="size-3.5" />
            {{ copiedKey === 'sec-rss' ? '已复制 RSS' : '复制带 Token 的 RSS 地址' }}
          </button>
          <button type="button" class="btn !py-1 text-xs" @click="copyText('sec-pod', secretPodcastUrl)">
            <component :is="copiedKey === 'sec-pod' ? Check : Copy" class="size-3.5" />
            {{ copiedKey === 'sec-pod' ? '已复制 Podcast' : '复制带 Token 的 Podcast 地址' }}
          </button>
        </div>
      </div>

      <form class="flex flex-wrap items-end gap-2" @submit.prevent="createMut.mutate()">
        <div class="min-w-48 flex-1">
          <label class="label">新增只读 Token 备注（如阅读器 / 成员名）</label>
          <input v-model="label" class="input" maxlength="100" placeholder="例如：Reeder / 团队成员 A" required />
        </div>
        <button class="btn btn-primary" :disabled="createMut.isPending.value || !label.trim()">
          <Plus class="size-4" />生成 Token
        </button>
      </form>

      <p v-if="activeErr" class="text-sm text-red-400">{{ errorMessage(activeErr) }}</p>

      <div>
        <h3 class="label">已签发的只读 Token（{{ tokens.length }}）</h3>
        <p v-if="tokensQuery.isPending.value" class="py-4 text-sm text-gray-500">加载中…</p>
        <p v-else-if="!tokens.length" class="py-4 text-sm text-gray-500">当前目录还没有签发只读 Token。</p>
        <ul v-else class="divide-y divide-gray-700 rounded-lg border border-gray-700 bg-gray-900/50">
          <li v-for="t in tokens" :key="t.id" class="flex flex-wrap items-center justify-between gap-2 px-3 py-2.5 text-sm">
            <div class="min-w-0">
              <p class="flex items-center gap-1.5 font-medium text-gray-200"><KeyRound class="size-3.5 text-amber-300" />{{ t.label }}</p>
              <p class="text-xs text-gray-500">ID：{{ t.id }} · 创建于 {{ fmtDate(t.created_at) }}</p>
            </div>
            <div class="flex items-center gap-2">
              <button type="button" class="btn !py-1 text-xs" @click="confirmTarget = { kind: 'rotate', token: t }">
                <RefreshCw class="size-3.5" />轮换
              </button>
              <button type="button" class="btn btn-danger !py-1 text-xs" @click="confirmTarget = { kind: 'delete', token: t }">
                <Trash2 class="size-3.5" />吊销
              </button>
            </div>
          </li>
        </ul>
      </div>
    </div>

    <ConfirmDialog
      :open="!!confirmTarget"
      :title="confirmTarget?.kind === 'rotate' ? '轮换目录 Token' : '吊销目录 Token'"
      :description="confirmTarget?.kind === 'rotate'
        ? `确定轮换「${confirmTarget.token.label}」？轮换后旧 Token 将立即失效，需要更新订阅链接。`
        : `确定吊销「${confirmTarget?.token.label ?? ''}」？使用该 Token 的订阅源和代理请求将立即返回 401。`"
      :confirm-text="confirmTarget?.kind === 'rotate' ? '确认轮换' : '确认吊销'"
      :pending="rotateMut.isPending.value || deleteMut.isPending.value"
      @update:open="(o) => !o && (confirmTarget = null)"
      @confirm="handleConfirm"
    />
  </Modal>
</template>
