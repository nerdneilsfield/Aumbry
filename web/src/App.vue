<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'
</script>

<template>
  <div class="shell">
    <header class="topbar">
      <RouterLink class="brand" to="/">Aumbry</RouterLink>
      <span class="tagline">Research, gathered and searchable.</span>
    </header>
    <main>
      <RouterView />
    </main>
  </div>
</template>
