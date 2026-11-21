<script setup lang="ts">
import { computed } from 'vue'
import { ArrowRight, Compass } from 'lucide-vue-next'
import { attr, children } from '@/lib/xml'
import { useAnchors } from '@/composables/useAnchors'

const props = defineProps<{ node: Element }>()
const { canTarget, navigate } = useAnchors()

// Targets are same-document anchors by id only: `^#[a-z0-9-]+$` resolving through the prefix table (#s- #stages- #stage- #quiz-
// #fig- #eq- #ref-) to an element that exists. Not URLs, so there is no safeUrl here: anything else (INVALID_TARGET) is shown disabled.
const items = computed(() =>
  children(props.node)
    .filter((c) => c.tagName === 'option')
    .map((o) => {
      const target = attr(o, 'target').trim()
      const ok = canTarget(target)
      return { label: o.textContent?.trim() ?? '', target, ok }
    }),
)
</script>

<template>
  <nav class="not-prose print:hidden" aria-label="接下来">
    <p class="mb-2 flex items-center gap-1.5 text-xs font-semibold text-gray-400"><Compass class="size-3.5" aria-hidden="true" />接下来你想…</p>
    <div class="flex flex-wrap gap-2">
      <button
        v-for="(it, i) in items"
        :key="i"
        type="button"
        :disabled="!it.ok"
        :title="it.ok ? it.target : 'INVALID_TARGET：目标不存在或不是有效的锚点 id'"
        :data-target="it.target"
        class="btn !justify-start text-left disabled:cursor-not-allowed"
        :class="i === 0 && it.ok ? 'btn-primary' : ''"
        @click="navigate(it.target)"
      >
        <span class="min-w-0 break-words">{{ it.label }}</span><ArrowRight class="size-4 shrink-0" aria-hidden="true" />
      </button>
    </div>
  </nav>
</template>
