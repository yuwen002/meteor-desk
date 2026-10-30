// 依赖图只做布局计算：节点按最长路径分层，边从被依赖方指向依赖方，纯函数便于单测。

export interface GraphInput { id:string; name:string; dependsOn?:string[] }
export interface GraphNode { id:string; name:string; dependsOn:string[]; layer:number; x:number; y:number; width:number; height:number }
export interface GraphEdge { from:string; to:string; path:string; cyclic:boolean }
export interface DependencyGraph {
  nodes:GraphNode[]
  edges:GraphEdge[]
  layers:number
  width:number
  height:number
  cyclic:string[]
}

export const NODE_WIDTH = 148
export const NODE_HEIGHT = 34
const GAP_X = 56
const GAP_Y = 12
const PADDING = 12

// 沿「被依赖 → 依赖方」方向做可达性搜索，用于判定回边与循环依赖。
function reachable(start:string, children:Map<string,string[]>):Set<string> {
  const seen = new Set<string>()
  const queue = [...(children.get(start) ?? [])]
  while (queue.length) {
    const id = queue.shift() as string
    if (id === start || seen.has(id)) continue
    seen.add(id)
    for (const next of children.get(id) ?? []) if (!seen.has(next)) queue.push(next)
  }
  return seen
}

// buildDependencyGraph 把服务配置排成从左到右的分层图；未知依赖与自依赖会被忽略。
export function buildDependencyGraph(services:GraphInput[]):DependencyGraph {
  if (!services.length) return { nodes: [], edges: [], layers: 0, width: 0, height: 0, cyclic: [] }

  const known = new Set(services.map(item => item.id))
  const deps = new Map<string,string[]>()
  const children = new Map<string,string[]>()
  for (const service of services) {
    const seen = new Set<string>()
    const list: string[] = []
    for (const dep of service.dependsOn ?? []) {
      if (dep === service.id || !known.has(dep) || seen.has(dep)) continue
      seen.add(dep)
      list.push(dep)
      children.set(dep, [...(children.get(dep) ?? []), service.id])
    }
    deps.set(service.id, list)
  }

  // 循环 = 边的两端互相可达；循环边不参与分层，避免深度无限增长。
  const reach = new Map<string,Set<string>>()
  for (const service of services) reach.set(service.id, reachable(service.id, children))
  const cyclicEdges = new Set<string>()
  const cyclicNodes = new Set<string>()
  for (const service of services) {
    for (const dep of deps.get(service.id) ?? []) {
      if (!reach.get(service.id)?.has(dep)) continue
      cyclicEdges.add(dep + '>' + service.id)
      cyclicNodes.add(service.id)
      cyclicNodes.add(dep)
    }
  }

  // 最长路径分层：按服务数量轮数做松弛，循环边跳过后必然收敛。
  const depth = new Map<string,number>(services.map(item => [item.id, 0]))
  for (let round = 0; round < services.length; round++) {
    let changed = false
    for (const service of services) {
      for (const dep of deps.get(service.id) ?? []) {
        if (cyclicEdges.has(dep + '>' + service.id)) continue
        const candidate = (depth.get(dep) ?? 0) + 1
        if (candidate > (depth.get(service.id) ?? 0) && candidate < services.length) {
          depth.set(service.id, candidate)
          changed = true
        }
      }
    }
    if (!changed) break
  }

  // 同层节点保持配置列表顺序，位置稳定便于对照。
  const rows = new Map<number,number>()
  const nodes:GraphNode[] = services.map(service => {
    const layer = depth.get(service.id) ?? 0
    const row = rows.get(layer) ?? 0
    rows.set(layer, row + 1)
    return {
      id: service.id,
      name: service.name,
      dependsOn: [...(deps.get(service.id) ?? [])],
      layer,
      x: PADDING + layer * (NODE_WIDTH + GAP_X),
      y: PADDING + row * (NODE_HEIGHT + GAP_Y),
      width: NODE_WIDTH,
      height: NODE_HEIGHT,
    }
  })

  const byID = new Map(nodes.map(node => [node.id, node]))
  const edges:GraphEdge[] = []
  for (const node of nodes) {
    for (const dep of node.dependsOn) {
      const from = byID.get(dep)
      if (!from) continue
      const x1 = from.x + from.width
      const y1 = from.y + from.height / 2
      const x2 = node.x
      const y2 = node.y + node.height / 2
      // 正向边用对称三次曲线；回边从下方绕回，避免与正向边完全重叠。
      const midX = (x1 + x2) / 2
      const path = x2 > x1
        ? `M ${x1} ${y1} C ${midX} ${y1}, ${midX} ${y2}, ${x2} ${y2}`
        : `M ${x1} ${y1} C ${x1 + GAP_X / 2} ${y1 + NODE_HEIGHT}, ${x2 - GAP_X / 2} ${y2 + NODE_HEIGHT}, ${x2} ${y2}`
      edges.push({ from: dep, to: node.id, path, cyclic: cyclicEdges.has(dep + '>' + node.id) })
    }
  }

  const layers = Math.max(...nodes.map(node => node.layer)) + 1
  const maxRows = Math.max(...[...rows.values()])
  return {
    nodes,
    edges,
    layers,
    width: PADDING * 2 + layers * NODE_WIDTH + (layers - 1) * GAP_X,
    height: PADDING * 2 + maxRows * NODE_HEIGHT + (maxRows - 1) * GAP_Y,
    cyclic: [...cyclicNodes],
  }
}
