<script setup lang="ts">
import { useRoute } from 'vue-router'

const route = useRoute()
</script>

<template>
  <article class="card">
    <p class="eyebrow">Report</p>
    <h1>{{ route.params.id }}</h1>
    <p class="muted">Report metadata and XML rendering are not implemented yet.</p>
  </article>
</template>
