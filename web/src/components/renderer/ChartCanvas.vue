<script setup lang="ts">
// The only file that imports echarts / vue-echarts. ChartBlock loads it through defineAsyncComponent,
// so the library is a lazy chunk that documents without <chart> never fetch.
import { use, type EChartsCoreOption } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { BarChart, LineChart, PieChart, ScatterChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import VChart from 'vue-echarts'

use([CanvasRenderer, BarChart, LineChart, PieChart, ScatterChart, GridComponent, LegendComponent, TooltipComponent])

defineProps<{ option: EChartsCoreOption; height: number; label: string }>()
</script>

<template>
  <VChart :option="option" autoresize role="img" :aria-label="label" :style="{ height: `${height}px`, width: '100%' }" />
</template>
