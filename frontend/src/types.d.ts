export {}

declare global {
  interface LogMatch {
    lineNumber: number
    content: string
    contentClipped: boolean
  }
  interface LogSearchResponse {
    filePath: string
    fileSizeBytes: number
    scannedLines: number
    matches: LogMatch[]
    resultLimitReached: boolean
  }
  interface Window {
    go?: {
      main?: {
        App?: {
          SelectLogFile: () => Promise<string>
          SearchLogs: (path: string, query: string, useRegex: boolean, maxResults: number) => Promise<LogSearchResponse>
        }
      }
    }
  }
}
