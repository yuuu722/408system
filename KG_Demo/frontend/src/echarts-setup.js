import * as echarts from 'echarts/core'
import { GraphChart } from 'echarts/charts'
import {
  TooltipComponent,
  TitleComponent,
  LegendComponent
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

echarts.use([
  GraphChart,
  TooltipComponent,
  TitleComponent,
  LegendComponent,
  CanvasRenderer
])

export const $echarts = echarts
