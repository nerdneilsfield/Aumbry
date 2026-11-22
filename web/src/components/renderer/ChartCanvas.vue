<script setup lang="ts">
// The only file that imports echarts / vue-echarts. ChartBlock loads it through defineAsyncComponent,
// so the library is a lazy chunk that documents without <chart> never fetch.
import { shallowRef, useTemplateRef, watch } from 'vue'
import { use, type EChartsCoreOption } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { BarChart, CustomChart, LineChart, PieChart, ScatterChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import VChart from 'vue-echarts'
import { usePrinting } from '@/composables/usePrint'

use([CanvasRenderer, BarChart, CustomChart, LineChart, PieChart, ScatterChart, GridComponent, LegendComponent, TooltipComponent])

defineProps<{ option: EChartsCoreOption; height: number; label: string }>()
// `ready`: drawn, animation included, so printing can wait for it
const emit = defineEmits<{ ready: [] }>()

// A canvas has a fixed pixel size and does not follow the paper width; on paper the chart is shown as a picture of itself that
// scales with the page (taken when drawing finishes while printing, and when printing starts on an already drawn chart).
const printing = usePrinting()
const chart = useTemplateRef<InstanceType<typeof VChart>>('chart')
const picture = shallowRef('')
const snapshot = () => (picture.value = printing.value ? (chart.value?.getDataURL({ type: 'png', pixelRatio: 2, backgroundColor: '#fff' }) ?? '') : '')
watch(printing, snapshot, { flush: 'post' })
const finished = () => {
  snapshot()
  emit('ready')
}
</script>

<template>
  <VChart ref="chart" :option="option" autoresize role="img" :aria-label="label" :class="picture && 'print:hidden!'" :style="{ height: `${height}px`, width: '100%' }" @finished="finished" />
  <img v-if="picture" :src="picture" :alt="label" class="hidden h-auto w-full print:block" />
</template>
