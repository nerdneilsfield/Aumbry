import * as v from 'valibot'
// Type-only import: erased at build time, so echarts stays out of the main chunks.
import type { EChartsCoreOption } from 'echarts/core'

/**
 * <chart> payload, PRD 6.3. We never pass the document's JSON to ECharts as-is: it is validated against this
 * schema and an option object is built from the checked fields only (no formatter functions, no raw option).
 */
const Num = v.pipe(v.number(), v.finite())
const Series = (data: v.GenericSchema) =>
  v.pipe(
    v.array(v.object({ name: v.pipe(v.string(), v.nonEmpty('series[].name 不能为空')), data: v.array(data) })),
    v.minLength(1, 'series 至少 1 个'),
    v.maxLength(20, 'series 最多 20 个'),
  )
const labels = { x_label: v.optional(v.string()), y_label: v.optional(v.string()) }

const Categorical = v.pipe(
  v.object({ categories: v.pipe(v.array(v.string()), v.nonEmpty('categories 不能为空')), series: Series(Num), ...labels }),
  v.check((d) => d.series.every((s) => s.data.length === d.categories.length), 'series[].data 的长度必须等于 categories 的长度'),
  v.check((d) => d.series.reduce((n, s) => n + s.data.length, 0) <= 2000, '数据点总数不能超过 2000'),
)
const Scatter = v.pipe(
  v.object({ series: Series(v.tuple([Num, Num])), ...labels }),
  v.check((d) => d.series.reduce((n, s) => n + s.data.length, 0) <= 2000, '数据点总数不能超过 2000'),
)

export type ChartType = 'bar' | 'line' | 'pie' | 'scatter'
export const isChartType = (t: string): t is ChartType => ['bar', 'line', 'pie', 'scatter'].includes(t)

type CategoricalData = v.InferOutput<typeof Categorical>
type ScatterData = v.InferOutput<typeof Scatter>

export interface ChartSpec {
  option: EChartsCoreOption
  /** px; known before echarts loads so the placeholder reserves exactly this space */
  height: number
}

/** Parses + validates; returns a ready-to-render spec or a human readable error (with JSON path of the first issue). */
export function buildChart(type: string, json: string, dark: boolean): ChartSpec | { error: string } {
  if (!isChartType(type)) return { error: `不支持的图表类型 “${type}”（应为 bar / line / pie / scatter）` }
  let raw: unknown
  try {
    raw = JSON.parse(json)
  } catch (e) {
    return { error: `JSON 解析失败：${e instanceof Error ? e.message : String(e)}` }
  }
  const r = v.safeParse(type === 'scatter' ? Scatter : Categorical, raw)
  if (!r.success) {
    const i = r.issues[0]
    return { error: `数据不符合格式：${v.getDotPath(i) ?? '$'} — ${i.message}` }
  }
  return type === 'scatter' ? scatterOption(r.output as ScatterData, dark) : categoricalOption(type, r.output as CategoricalData, dark)
}

const PALETTE = ['#3b82f6', '#10b981', '#f59e0b', '#a855f7', '#ef4444', '#06b6d4', '#ec4899', '#84cc16']

function tone(dark: boolean) {
  return {
    text: dark ? '#d1d5db' : '#374151',
    muted: dark ? '#9ca3af' : '#4b5563',
    axis: dark ? '#4b5563' : '#9ca3af',
    split: dark ? '#1f2937' : '#e5e7eb',
    tipBg: dark ? '#111827' : '#ffffff',
    tipBorder: dark ? '#374151' : '#d1d5db',
  }
}

function base(dark: boolean, trigger: 'axis' | 'item', legend: boolean) {
  const t = tone(dark)
  return {
    color: PALETTE,
    backgroundColor: 'transparent',
    textStyle: { color: t.text, fontFamily: 'Inter, ui-sans-serif, system-ui, "PingFang SC", "Noto Sans SC", sans-serif' },
    // richText: the tooltip is drawn on the canvas as text. No HTML string is ever built from series names or categories.
    tooltip: {
      trigger,
      renderMode: 'richText',
      confine: true,
      backgroundColor: t.tipBg,
      borderColor: t.tipBorder,
      textStyle: { color: t.text, fontSize: 12 },
    },
    legend: legend ? { type: 'scroll', top: 0, textStyle: { color: t.text }, pageTextStyle: { color: t.text }, pageIconColor: t.text } : undefined,
  } satisfies EChartsCoreOption
}

function categoricalOption(type: 'bar' | 'line' | 'pie', d: CategoricalData, dark: boolean): ChartSpec {
  const t = tone(dark)
  const n = d.categories.length
  const longest = Math.max(...d.categories.map((c) => [...c].length))

  if (type === 'pie') {
    const s = d.series[0]
    return {
      height: 360,
      option: {
        ...base(dark, 'item', false),
        legend: { type: 'scroll', bottom: 0, textStyle: { color: t.text }, pageTextStyle: { color: t.text }, pageIconColor: t.text },
        series: [{
          type: 'pie',
          name: s.name,
          radius: ['36%', '64%'],
          center: ['50%', '45%'],
          avoidLabelOverlap: true,
          itemStyle: { borderColor: dark ? '#111827' : '#ffffff', borderWidth: 2, borderRadius: 4 },
          label: { color: t.text, formatter: '{b}\n{d}%', width: 120, overflow: 'truncate' },
          labelLine: { lineStyle: { color: t.axis } },
          data: d.categories.map((name, i) => ({ name, value: s.data[i] })),
        }],
      },
    }
  }

  // Horizontal bars keep long category names readable; the full name is in the tooltip.
  const horizontal = type === 'bar' && (longest > 10 || n > 8)
  const catAxis = {
    type: 'category' as const,
    data: d.categories,
    inverse: horizontal,
    axisLine: { lineStyle: { color: t.axis } },
    axisTick: { show: false },
    axisLabel: horizontal
      ? { color: t.text, width: 130, overflow: 'truncate' as const, interval: 0 }
      : { color: t.text, interval: 0, width: n > 6 ? 70 : 110, overflow: n > 6 ? ('truncate' as const) : ('break' as const), rotate: n > 6 ? 30 : 0 },
    name: horizontal ? undefined : d.x_label,
    nameLocation: 'middle' as const,
    nameGap: n > 6 ? 62 : 34,
    nameTextStyle: { color: t.muted },
  }
  const valAxis = {
    type: 'value' as const,
    axisLabel: { color: t.muted },
    splitLine: { lineStyle: { color: t.split } },
    name: horizontal ? d.x_label : d.y_label,
    nameLocation: 'end' as const,
    nameTextStyle: { color: t.muted, align: horizontal ? ('right' as const) : ('left' as const) },
  }
  const labelled = d.series.length === 1 && n <= 12
  const series = d.series.map((s) => ({
    type,
    name: s.name,
    data: s.data,
    ...(type === 'line' ? { smooth: false, symbolSize: 7, lineStyle: { width: 2.5 } } : { barMaxWidth: 36, itemStyle: { borderRadius: horizontal ? [0, 4, 4, 0] : [4, 4, 0, 0] } }),
    label: labelled ? { show: true, position: horizontal ? ('right' as const) : ('top' as const), color: t.text, fontSize: 11 } : { show: false },
  }))

  return {
    height: horizontal ? Math.max(260, 90 + n * 34 * d.series.length) : n > 6 ? 410 : 350,
    option: {
      ...base(dark, 'axis', d.series.length > 1),
      grid: { top: d.series.length > 1 ? 52 : 36, left: 12, right: horizontal ? 36 : 20, bottom: !horizontal && d.x_label ? (n > 6 ? 40 : 28) : 12, containLabel: true },
      xAxis: horizontal ? valAxis : catAxis,
      yAxis: horizontal ? catAxis : valAxis,
      series,
    },
  }
}

function scatterOption(d: ScatterData, dark: boolean): ChartSpec {
  const t = tone(dark)
  const axis = (name: string | undefined, loc: 'middle' | 'end') => ({
    type: 'value' as const,
    scale: true,
    name,
    nameLocation: loc,
    nameGap: 30,
    nameTextStyle: { color: t.muted, align: loc === 'end' ? ('left' as const) : ('center' as const) },
    axisLine: { lineStyle: { color: t.axis } },
    axisLabel: { color: t.muted },
    splitLine: { lineStyle: { color: t.split } },
  })
  return {
    height: 360,
    option: {
      ...base(dark, 'item', d.series.length > 1),
      grid: { top: d.series.length > 1 ? 64 : 40, left: 12, right: 24, bottom: d.x_label ? 30 : 12, containLabel: true },
      xAxis: axis(d.x_label, 'middle'),
      yAxis: axis(d.y_label, 'end'),
      series: d.series.map((s) => ({ type: 'scatter' as const, name: s.name, data: s.data, symbolSize: 11, itemStyle: { opacity: 0.85 } })),
    },
  }
}
