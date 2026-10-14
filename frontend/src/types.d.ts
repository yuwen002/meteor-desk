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
    bytesScanned: number
    scannedLines: number
    matches: LogMatch[]
    resultLimitReached: boolean
    canceled: boolean
  }

  interface LogSearchProgress {
    searchId: string
    bytesScanned: number
    fileSizeBytes: number
    scannedLines: number
    matchesFound: number
    percent: number
    running: boolean
    canceled: boolean
    error?: string
  }

  interface Window {
    go?: {
      main?: {
        App?: {
          SelectLogFile: () => Promise<string>
          SearchLogs: (searchId: string, path: string, query: string, useRegex: boolean, maxResults: number) => Promise<LogSearchResponse>
          GetLogSearchProgress: (searchId: string) => Promise<LogSearchProgress>
          CancelLogSearch: (searchId: string) => Promise<boolean>
        }
      }
    }
  }
}
