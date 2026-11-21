<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { TreeItem, TreeRoot } from 'reka-ui'
import { ChevronRight, Folder, Lock, Square, SquareCheck } from 'lucide-vue-next'
import type { DirNode } from '@/api/types'
import { findNode, flattenTree } from '@/lib/tree'

const props = defineProps<{ items: DirNode[]; multiple?: boolean }>()
// Selected directory ids (single mode keeps at most one).
const model = defineModel<string[]>({ default: () => [] })

const selected = computed(() => model.value.flatMap((id) => findNode(props.items, id) ?? []))

// Expand everything, including directories that appear later.
const seen = new Set<string>()
const expanded = ref<string[]>([])
watch(
  () => props.items,
  (items) => {
    const fresh = flattenTree(items).map((d) => d.id).filter((id) => !seen.has(id))
    fresh.forEach((id) => seen.add(id))
    expanded.value = [...expanded.value, ...fresh]
  },
  { immediate: true },
)

function update(v: unknown) {
  model.value = ([v].flat() as (DirNode | undefined)[]).flatMap((n) => n?.id ?? [])
}
</script>

<template>
  <TreeRoot
    v-slot="{ flattenItems }"
    :items="items"
    :get-key="(n: DirNode) => n.id"
    :get-children="(n: DirNode) => (n.children.length ? n.children : undefined)"
    v-model:expanded="expanded"
    :multiple="multiple"
    :model-value="multiple ? selected : selected[0]"
    class="space-y-0.5"
    @update:model-value="update"
  >
    <TreeItem v-for="item in flattenItems" :key="item._id" v-slot="{ isSelected, isExpanded, handleToggle }" v-bind="item.bind" class="outline-none">
      <div
        :class="['flex cursor-pointer items-center gap-1.5 rounded-lg py-1.5 pr-2 text-sm transition', isSelected && !multiple ? 'bg-blue-500/15 text-blue-200' : 'text-gray-300 hover:bg-gray-700/60']"
        :style="{ paddingLeft: `${(item.level - 1) * 1 + 0.25}rem` }"
      >
        <button type="button" tabindex="-1" :class="['shrink-0 text-gray-500', !item.hasChildren && 'invisible']" @click.stop="handleToggle">
          <ChevronRight :class="['size-4 transition-transform', isExpanded && 'rotate-90']" />
        </button>
        <component :is="isSelected ? SquareCheck : Square" v-if="multiple" class="size-4 shrink-0 text-blue-300" />
        <Folder v-else class="size-4 shrink-0 text-gray-500" />
        <span class="min-w-0 flex-1 truncate">{{ item.value.name }}</span>
        <Lock v-if="item.value.has_password" class="size-3.5 shrink-0 text-amber-300" />
        <span v-if="!multiple" class="shrink-0 text-xs text-gray-500">{{ item.value.item_count }}</span>
      </div>
    </TreeItem>
  </TreeRoot>
</template>
