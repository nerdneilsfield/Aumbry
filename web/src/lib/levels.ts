import { BadgeCheck, CircleHelp, Lightbulb, Megaphone, Ban } from 'lucide-vue-next'
import type { Component } from 'vue'

export type Level = 'claim' | 'evidence' | 'inference' | 'unverified' | 'na'

interface LevelStyle {
  label: string
  hint: string
  icon: Component
  /** left bar / border colour */
  bar: string
  /** badge: tinted bg + text; text colours come from the flipped palette so they stay readable in light mode */
  badge: string
  /** subtle tinted surface */
  surface: string
}

// Colour is never the only signal: each level also has its own icon and label.
export const LEVELS: Record<Level, LevelStyle> = {
  claim: {
    label: '作者主张',
    hint: '论文作者自己的陈述',
    icon: Megaphone,
    bar: 'border-l-blue-400',
    badge: 'bg-blue-500/15 text-blue-300 ring-blue-400/40',
    surface: 'bg-blue-500/5',
  },
  evidence: {
    label: '论文证据',
    hint: '实验、表格或图中的直接结果',
    icon: BadgeCheck,
    bar: 'border-l-emerald-400',
    badge: 'bg-emerald-500/15 text-emerald-300 ring-emerald-400/40',
    surface: 'bg-emerald-500/5',
  },
  inference: {
    label: '分析推断',
    hint: '读者或 Agent 的推理，不是论文原话',
    icon: Lightbulb,
    bar: 'border-l-violet-400',
    badge: 'bg-violet-500/15 text-violet-300 ring-violet-400/40',
    surface: 'bg-violet-500/5',
  },
  unverified: {
    label: '待核验',
    hint: '论文未报告或尚未核实',
    icon: CircleHelp,
    bar: 'border-l-amber-400 border-dashed',
    badge: 'bg-amber-500/15 text-amber-300 ring-amber-400/40',
    surface: 'bg-amber-500/5',
  },
  na: {
    label: '不可比',
    hint: '协议或口径不同，不能直接比较',
    icon: Ban,
    bar: 'border-l-gray-500',
    badge: 'bg-gray-500/20 text-gray-300 ring-gray-500/40',
    surface: 'bg-gray-500/10',
  },
}

export const asLevel = (raw: string): Level | undefined => (raw in LEVELS ? (raw as Level) : undefined)
