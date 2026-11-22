<script setup lang="ts">
import { computed } from 'vue'
import { useExpanded } from '@/composables/usePrint'
import { TabsContent, TabsList, TabsRoot, TabsTrigger } from 'reka-ui'
import { attr, children } from '@/lib/xml'
import { tabKey, useAnchors } from '@/composables/useAnchors'
import XmlNode from './XmlNode.vue'

const props = defineProps<{ node: Element }>()
const { activeTabs } = useAnchors()
// Printing / export: forceMount keeps every tab's content mounted (and shown).
const expanded = useExpanded()
const tabs = computed(() => children(props.node).filter((c) => c.tagName === 'tab'))

// The active tab lives in the shared anchor state so that #fig-* / #eq-* / #ref-* links can switch it.
const active = computed({
  get: () => activeTabs.get(props.node) ?? (tabs.value[0] ? tabKey(tabs.value[0]) : ''),
  set: (k) => activeTabs.set(props.node, k),
})
</script>

<template>
  <TabsRoot v-model="active" class="not-prose card overflow-hidden">
    <TabsList class="flex gap-1 overflow-x-auto border-b border-gray-700 bg-gray-900/60 px-2 pt-2" aria-label="标签页">
      <TabsTrigger
        v-for="t in tabs"
        :key="tabKey(t)"
        :value="tabKey(t)"
        class="-mb-px shrink-0 cursor-pointer whitespace-nowrap rounded-t-lg border border-transparent px-4 py-2 text-sm font-medium text-gray-400 outline-none transition hover:text-gray-100 focus-visible:ring-2 focus-visible:ring-blue-400 data-[state=active]:border-gray-700 data-[state=active]:border-b-gray-800 data-[state=active]:bg-gray-800 data-[state=active]:text-blue-300"
      >{{ attr(t, 'title') }}</TabsTrigger>
    </TabsList>
    <TabsContent v-for="t in tabs" :key="tabKey(t)" :value="tabKey(t)" :force-mount="expanded" :data-export-title="attr(t, 'title')" class="space-y-4 p-4 outline-none print:block!">
      <XmlNode v-for="(n, i) in children(t)" :key="i" :node="n" />
    </TabsContent>
  </TabsRoot>
</template>
