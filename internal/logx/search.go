// Package logx contains streaming log-file search primitives.
package logx

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	defaultMaxResults = 500
	hardMaxResults    = 5000
	maxPreviewBytes   = 8 * 1024
)

type Match struct {
	LineNumber     int64  `json:"lineNumber"`
	Content        string `json:"content"`
	ContentClipped bool   `json:"contentClipped"`
}

type SearchResponse struct {
	FilePath           string  `json:"filePath"`
	FileSizeBytes      int64   `json:"fileSizeBytes"`
	ScannedLines       int64   `json:"scannedLines"`
	Matches            []Match `json:"matches"`
	ResultLimitReached bool    `json:"resultLimitReached"`
}

// SearchFile scans sequentially without loading the whole file at once. Results and
// previews are capped. A single exceptionally long line can still require temporary
// memory while assembled; chunk-safe matching for that edge case is planned.
func SearchFile(path, query string, useRegex bool, maxResults int) (SearchResponse, error) {
	if strings.TrimSpace(path) == "" { return SearchResponse{}, errors.New("choose a log file first") }
	if query == "" { return SearchResponse{}, errors.New("enter a search term") }
	if maxResults <= 0 { maxResults = defaultMaxResults }
	if maxResults > hardMaxResults { maxResults = hardMaxResults }

	var matcher *regexp.Regexp
	if useRegex {
		compiled, err := regexp.Compile(query)
		if err != nil { return SearchResponse{}, fmt.Errorf("invalid regular expression: %w", err) }
		matcher = compiled
	}

	file, err := os.Open(path)
	if err != nil { return SearchResponse{}, fmt.Errorf("open log file: %w", err) }
	defer file.Close()
	info, err := file.Stat()
	if err != nil { return SearchResponse{}, fmt.Errorf("stat log file: %w", err) }

	response := SearchResponse{
		FilePath: path, FileSizeBytes: info.Size(),
		Matches: make([]Match, 0, min(maxResults, 128)),
	}
	reader := bufio.NewReaderSize(file, 256*1024)
	for {
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			response.ScannedLines++
			line = strings.TrimSuffix(line, "\n")
			line = strings.TrimSuffix(line, "\r")
			matched := strings.Contains(line, query)
			if matcher != nil { matched = matcher.MatchString(line) }
			if matched {
				preview, clipped := clipLine(line, maxPreviewBytes)
				response.Matches = append(response.Matches, Match{
					LineNumber: response.ScannedLines, Content: preview, ContentClipped: clipped,
				})
				if len(response.Matches) >= maxResults {
					response.ResultLimitReached = true
					return response, nil
				}
			}
		}
		if readErr == io.EOF { break }
		if readErr != nil { return SearchResponse{}, fmt.Errorf("read log file: %w", readErr) }
	}
	return response, nil
}

func clipLine(line string, limit int) (string, bool) {
	if len(line) <= limit { return line, false }
	end := limit
	for end > 0 && !utf8.ValidString(line[:end]) { end-- }
	return line[:end] + "… [line clipped]", true
}
