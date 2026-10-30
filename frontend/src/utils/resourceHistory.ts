// 资源曲线的纯计算逻辑：只做归一化与峰值统计，便于脱离组件单元测试。

export interface ResourcePointLike { cpuPercent?:number; memoryBytes?:number }

export interface ResourceHistorySummary {
  cpuValues:number[]
  memoryValues:number[]
  peakCPU:number
  peakMemory:number
  memoryDelta:number
  count:number
}

// summarizeHistory 提取绘图所需序列并计算峰值；缺失或非法值按 0 处理。
export function summarizeHistory(points:ResourcePointLike[]):ResourceHistorySummary {
  const cpuValues=points.map(point=>Number.isFinite(point.cpuPercent)?Number(point.cpuPercent):0)
  const memoryValues=points.map(point=>Number.isFinite(point.memoryBytes)?Number(point.memoryBytes):0)
  const peakCPU=cpuValues.length?Math.max(...cpuValues):0
  const peakMemory=memoryValues.length?Math.max(...memoryValues):0
  const memoryDelta=memoryValues.length>1?memoryValues[memoryValues.length-1]-memoryValues[0]:0
  return {cpuValues,memoryValues,peakCPU,peakMemory,memoryDelta,count:points.length}
}

// sparklinePoints 把数值序列缩放到 width×height 的 SVG 坐标，最小值为底边、最大值为顶边。
// 只有一个采样点时画一条中线，避免出现无法点击或不可见的单点。
export function sparklinePoints(values:number[],width:number,height:number):string {
  if(!values.length)return ''
  const mid=(height/2).toFixed(1)
  if(values.length===1)return '0,'+mid+' '+width+','+mid
  const max=Math.max(...values)
  const min=Math.min(...values)
  const span=max-min
  const step=width/(values.length-1)
  return values.map((value,index)=>{
    const x=(index*step).toFixed(1)
    const y=(span===0?height/2:height-((value-min)/span)*height).toFixed(1)
    return x+','+y
  }).join(' ')
}
