<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { Library, ShieldCheck } from 'lucide-vue-next'
</script>

<template>
  <section class="card mx-auto mt-10 max-w-2xl p-8 text-center sm:p-12">
    <Library class="mx-auto size-10 text-blue-300" />
    <h1 class="mt-5 text-3xl font-bold text-gray-50">Aumbry</h1>
    <p class="mt-3 text-gray-400">私有研究资料库。内容通过分享链接访问，这里不提供公开目录。</p>
    <p class="mt-1 text-gray-400">如果你收到了目录链接，请直接打开它。</p>
    <RouterLink to="/admin" class="btn mt-8"><ShieldCheck class="size-4" />管理员入口</RouterLink>
  </section>
</template>
