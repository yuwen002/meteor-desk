import { describe, expect, it } from 'vitest'
import { buildDependencyGraph, NODE_WIDTH } from './dependencyGraph'

const service = (id:string, dependsOn:string[] = []) => ({ id, name: id.toUpperCase(), dependsOn })

describe('buildDependencyGraph', () => {
  it('returns an empty graph without services', () => {
    const graph = buildDependencyGraph([])
    expect(graph.nodes).toEqual([])
    expect(graph.edges).toEqual([])
    expect(graph.layers).toBe(0)
    expect(graph.width).toBe(0)
    expect(graph.height).toBe(0)
    expect(graph.cyclic).toEqual([])
  })

  it('layers services by the longest dependency chain', () => {
    const graph = buildDependencyGraph([
      service('api', ['db', 'cache']),
      service('db'),
      service('cache'),
      service('web', ['api']),
    ])
    const byID = new Map(graph.nodes.map(node => [node.id, node]))
    expect(byID.get('db')!.layer).toBe(0)
    expect(byID.get('cache')!.layer).toBe(0)
    expect(byID.get('api')!.layer).toBe(1)
    expect(byID.get('web')!.layer).toBe(2)
    expect(graph.layers).toBe(3)
    expect(graph.edges.map(edge => edge.from + '>' + edge.to).sort()).toEqual(['api>web', 'cache>api', 'db>api'])
    expect(graph.width).toBeGreaterThan(NODE_WIDTH * 2)
    expect(graph.cyclic).toEqual([])
  })

  it('stacks siblings of the same layer on separate rows', () => {
    const graph = buildDependencyGraph([service('db'), service('cache')])
    expect(graph.nodes.map(node => node.layer)).toEqual([0, 0])
    expect(graph.nodes[1].y).toBeGreaterThan(graph.nodes[0].y)
    expect(graph.nodes[1].x).toBe(graph.nodes[0].x)
    expect(graph.height).toBeGreaterThan(graph.nodes[0].height)
  })

  it('marks cycle members and keeps the layout finite', () => {
    const graph = buildDependencyGraph([service('a', ['b']), service('b', ['a'])])
    expect([...graph.cyclic].sort()).toEqual(['a', 'b'])
    expect(graph.edges).toHaveLength(2)
    expect(graph.edges.every(edge => edge.cyclic)).toBe(true)
    expect(graph.layers).toBe(1)
  })

  it('ignores unknown, duplicate and self dependencies', () => {
    const graph = buildDependencyGraph([service('api', ['db', 'db', 'missing', 'api']), service('db')])
    expect(graph.nodes.find(node => node.id === 'api')!.dependsOn).toEqual(['db'])
    expect(graph.edges).toHaveLength(1)
    expect(graph.edges[0].path).toContain('C')
    expect(graph.cyclic).toEqual([])
  })
})
