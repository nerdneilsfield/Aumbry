<script setup lang="ts">
import { RouterLink } from 'vue-router'

/** A tag. With `dirId` it is a link that lists that directory's items carrying the tag; without, plain text. */
defineProps<{ tag: string; dirId?: string; active?: boolean }>()
</script>

<template>
  <RouterLink
    v-if="dirId"
    :to="{ path: `/d/${encodeURIComponent(dirId)}`, query: { tag } }"
    :aria-current="active ? 'true' : undefined"
    :title="`只看带「${tag}」标签的内容`"
    :class="['chip transition outline-none hover:bg-gray-600 focus-visible:ring-2 focus-visible:ring-blue-400', active && 'bg-blue-500/20 text-blue-200']"
  >{{ tag }}</RouterLink>
  <span v-else class="chip">{{ tag }}</span>
</template>
