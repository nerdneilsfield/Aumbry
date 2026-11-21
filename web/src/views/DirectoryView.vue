<script setup lang="ts">
import { useRoute } from 'vue-router'

const route = useRoute()
</script>

<template>
  <section class="card">
    <p class="eyebrow">Directory</p>
    <h1>{{ route.params.id }}</h1>
    <p class="muted">Directory API and password gate are not implemented yet.</p>
  </section>
</template>
