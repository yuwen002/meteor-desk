import { classifyLogLevel, type LogLevel } from './logLevel'
import { isWithinTimeRange, type TimeRange } from './timeRange'

export type SearchLogLevelFilter = 'ALL' | LogLevel

export function filterLogResults<T extends { content: string }>(
  matches: readonly T[],
  range: TimeRange,
  level: SearchLogLevelFilter,
): T[] {
  return matches.filter((match) =>
    isWithinTimeRange(match.content, range)
    && (level === 'ALL' || classifyLogLevel(match.content) === level),
  )
}
