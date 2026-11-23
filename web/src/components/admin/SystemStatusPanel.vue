<script setup lang="ts">
import { computed, ref } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { Activity, AlertTriangle, ChevronDown, ChevronRight, Cpu, Database, RefreshCw, Server, ShieldCheck } from 'lucide-vue-next'
import { api, errorMessage } from '@/api/client'
import { formatBytes, formatCpuPercent, formatUptime } from '@/lib/format'
import { formatBuildTime, formatVersionTag } from '@/lib/version'

const expanded = ref(true)
const autoRefresh = ref(false)

const statusQuery = useQuery({
  queryKey: ['admin', 'status'],
  queryFn: api.admin.status,
  refetchInterval: computed(() => (autoRefresh.value ? 10_000 : false)),
  staleTime: 0,
})

const data = computed(() => statusQuery.data.value)
const isHealthy = computed(() => data.value?.status === 'ok')
const primaryMem = computed(() => {
  const rt = data.value?.runtime
  if (!rt) return '—'
  return rt.process_rss_bytes ? formatBytes(rt.process_rss_bytes) : formatBytes(rt.heap_alloc_bytes)
})
const revLimitLabel = computed(() => {
  const maxRevs = data.value?.config.max_revisions_per_item ?? 0
  return maxRevs > 0 ? `${maxRevs} 版` : '不限制'
})
</script>

<template>
  <section data-testid="admin-system-status" class="card p-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="flex flex-wrap items-center gap-2.5 text-sm">
        <span
          v-if="data"
          :class="[
            'inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-medium',
            isHealthy
              ? 'border-emerald-500/40 bg-emerald-500/10 text-emerald-300'
              : 'border-amber-500/40 bg-amber-500/10 text-amber-300',
          ]"
        >
          <ShieldCheck v-if="isHealthy" class="size-3.5" aria-hidden="true" />
          <AlertTriangle v-else class="size-3.5" aria-hidden="true" />
          {{ isHealthy ? '运行正常' : '状态降级' }}
        </span>
        <span v-else class="chip">运行状态</span>

        <template v-if="data">
          <span class="chip font-mono">{{ formatVersionTag(data.build.version) }} · {{ data.build.commit }}</span>
          <span class="text-xs text-gray-400">
            SQLite <span :class="data.checks.sqlite === 'ok' ? 'text-emerald-300' : 'text-red-300'">{{ data.checks.sqlite || 'unknown' }}</span>
            · 存储 <span :class="data.checks.storage === 'ok' ? 'text-emerald-300' : 'text-red-300'">{{ data.checks.storage || 'unknown' }}</span>
          </span>
          <span class="hidden text-xs text-gray-400 sm:inline">· 运行 {{ formatUptime(data.runtime.uptime_seconds) }}</span>
          <span class="hidden text-xs text-gray-400 md:inline">· CPU {{ formatCpuPercent(data.runtime.cpu_percent) }}</span>
          <span class="hidden text-xs text-gray-400 md:inline">· 内存 {{ primaryMem }}</span>
          <span class="hidden text-xs text-gray-400 lg:inline">· 协程 {{ data.runtime.goroutines }}</span>
        </template>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <label class="flex cursor-pointer items-center gap-1.5 text-xs text-gray-400">
          <input v-model="autoRefresh" type="checkbox" class="accent-blue-500" />
          自动刷新 (10s)
        </label>
        <button type="button" class="btn !px-2.5 !py-1 text-xs" :disabled="statusQuery.isFetching.value" @click="statusQuery.refetch()">
          <RefreshCw :class="['size-3.5', statusQuery.isFetching.value ? 'animate-spin' : '']" aria-hidden="true" />
          刷新
        </button>
        <button type="button" class="btn !px-2.5 !py-1 text-xs" @click="expanded = !expanded">
          <ChevronDown v-if="expanded" class="size-3.5" aria-hidden="true" />
          <ChevronRight v-else class="size-3.5" aria-hidden="true" />
          {{ expanded ? '收起详情' : '展开详情' }}
        </button>
      </div>
    </div>

    <p v-if="statusQuery.error.value" class="mt-3 rounded border border-red-800/60 bg-red-950/30 px-3 py-2 text-xs text-red-300">
      无法加载系统状态：{{ errorMessage(statusQuery.error.value) }}
    </p>
    <p v-else-if="statusQuery.isPending.value" class="mt-3 text-xs text-gray-500">正在加载运行状态…</p>

    <div v-else-if="expanded && data" class="mt-3 grid gap-3 border-t border-gray-700/60 pt-3 sm:grid-cols-2 xl:grid-cols-4">
      <div class="rounded-lg border border-gray-700/60 bg-gray-900/50 p-3 text-xs">
        <div class="mb-2 flex items-center gap-1.5 font-medium text-gray-200">
          <Server class="size-3.5 text-blue-300" aria-hidden="true" />服务与版本
        </div>
        <dl class="space-y-1 text-gray-400">
          <div class="flex justify-between gap-2"><dt>版本</dt><dd class="font-mono text-gray-200">{{ formatVersionTag(data.build.version) }} ({{ data.build.commit }})</dd></div>
          <div class="flex justify-between gap-2"><dt>构建时间</dt><dd class="font-mono text-gray-200" :title="data.build.build_time">{{ formatBuildTime(data.build.build_time) }}</dd></div>
          <div class="flex justify-between gap-2"><dt>Go / 平台</dt><dd class="font-mono text-gray-200">{{ data.runtime.go_version }} · {{ data.runtime.goos }}/{{ data.runtime.goarch }}</dd></div>
          <div class="flex justify-between gap-2"><dt>运行时长</dt><dd class="font-mono text-gray-200" :title="data.runtime.started_at">{{ formatUptime(data.runtime.uptime_seconds) }}</dd></div>
        </dl>
      </div>

      <div class="rounded-lg border border-gray-700/60 bg-gray-900/50 p-3 text-xs">
        <div class="mb-2 flex items-center gap-1.5 font-medium text-gray-200">
          <Cpu class="size-3.5 text-emerald-300" aria-hidden="true" />CPU 与内存
        </div>
        <dl class="space-y-1 text-gray-400">
          <div class="flex justify-between gap-2"><dt>CPU 占用</dt><dd class="font-mono text-gray-200">{{ formatCpuPercent(data.runtime.cpu_percent) }} ({{ data.runtime.num_cpu }} 核)</dd></div>
          <div class="flex justify-between gap-2"><dt>累计 CPU 时间</dt><dd class="font-mono text-gray-200">{{ data.runtime.cpu_seconds_total.toFixed(2) }} s</dd></div>
          <div class="flex justify-between gap-2">
            <dt>{{ data.runtime.process_rss_bytes ? '进程 RSS' : 'Go Heap' }}</dt>
            <dd class="font-mono text-gray-200">{{ primaryMem }}</dd>
          </div>
          <div class="flex justify-between gap-2"><dt>Heap Alloc / Sys</dt><dd class="font-mono text-gray-200">{{ formatBytes(data.runtime.heap_alloc_bytes) }} / {{ formatBytes(data.runtime.sys_bytes) }}</dd></div>
          <div class="flex justify-between gap-2"><dt>Goroutines / GC</dt><dd class="font-mono text-gray-200">{{ data.runtime.goroutines }} / {{ data.runtime.gc_count }} 次</dd></div>
        </dl>
      </div>

      <div class="rounded-lg border border-gray-700/60 bg-gray-900/50 p-3 text-xs">
        <div class="mb-2 flex items-center gap-1.5 font-medium text-gray-200">
          <Database class="size-3.5 text-violet-300" aria-hidden="true" />数据与存储
        </div>
        <dl class="space-y-1 text-gray-400">
          <div class="flex justify-between gap-2"><dt>存储 / SQLite</dt><dd class="font-mono text-gray-200">{{ data.storage.backend }} · {{ formatBytes(data.storage.sqlite_bytes) }}</dd></div>
          <div class="flex justify-between gap-2"><dt>目录</dt><dd class="font-mono text-gray-200">{{ data.counts.directories }}（{{ data.counts.protected_directories }} 受保护）</dd></div>
          <div class="flex justify-between gap-2"><dt>条目</dt><dd class="font-mono text-gray-200">{{ data.counts.items }}（已发布 {{ data.counts.published_items }} / 草稿 {{ data.counts.draft_items }}）</dd></div>
          <div class="flex justify-between gap-2"><dt>修订 / 订阅 Token</dt><dd class="font-mono text-gray-200">{{ data.counts.revisions }} 版 / {{ data.counts.directory_tokens }} 个</dd></div>
          <div class="flex justify-between gap-2"><dt>媒体与附件资产</dt><dd class="font-mono text-gray-200">{{ data.counts.assets }} 个（{{ formatBytes(data.counts.asset_bytes) }}）</dd></div>
        </dl>
      </div>

      <div class="rounded-lg border border-gray-700/60 bg-gray-900/50 p-3 text-xs">
        <div class="mb-2 flex items-center gap-1.5 font-medium text-gray-200">
          <Activity class="size-3.5 text-amber-300" aria-hidden="true" />并发与配置限制
        </div>
        <dl class="space-y-1 text-gray-400">
          <div class="flex justify-between gap-2"><dt>写入并发</dt><dd class="font-mono text-gray-200">{{ data.activity.ingest_in_flight }} / {{ data.activity.ingest_limit }}</dd></div>
          <div class="flex justify-between gap-2"><dt>MCP 会话</dt><dd class="font-mono text-gray-200">{{ data.activity.mcp_active_sessions }} 活跃（{{ data.activity.mcp_anon_sessions }} 匿名）</dd></div>
          <div class="flex justify-between gap-2"><dt>单次写入上限</dt><dd class="font-mono text-gray-200">{{ formatBytes(data.config.max_ingest_body_bytes) }}</dd></div>
          <div class="flex justify-between gap-2"><dt>修订保留上限</dt><dd class="font-mono text-gray-200">{{ revLimitLabel }}</dd></div>
          <div class="flex justify-between gap-2"><dt>外链转存 / Public URL</dt><dd class="font-mono text-gray-200">{{ data.config.allow_remote_images ? '开启' : '关闭' }} / {{ data.config.public_url_configured ? '已配' : '未配' }}</dd></div>
        </dl>
      </div>
    </div>
  </section>
</template>
