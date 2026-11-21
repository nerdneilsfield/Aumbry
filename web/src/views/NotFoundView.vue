<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { FileQuestion } from 'lucide-vue-next'
import StatePage from '@/components/StatePage.vue'
</script>

<template>
  <StatePage :icon="FileQuestion" title="页面不存在" message="你访问的地址没有对应的内容。">
    <RouterLink to="/" class="btn">返回首页</RouterLink>
  </StatePage>
</template>
