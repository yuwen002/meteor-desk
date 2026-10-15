package logx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
	textunicode "golang.org/x/text/encoding/unicode"
)

func TestSearchFileLiteralAndLineNumbers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.log")
	if err := os.WriteFile(path, []byte("启动\nINFO 已登录\nWARN 超时\nINFO 就绪"), 0600); err != nil { t.Fatal(err) }
	got, err := SearchFile(path, "INFO", false, 10)
	if err != nil { t.Fatal(err) }
	if got.ScannedLines != 4 { t.Fatalf("扫描行数 = %d，期望 4", got.ScannedLines) }
	if len(got.Matches) != 2 { t.Fatalf("匹配数量 = %d，期望 2", len(got.Matches)) }
	if got.Matches[0].LineNumber != 2 || got.Matches[1].LineNumber != 4 { t.Fatalf("匹配行号异常：%+v", got.Matches) }
}

func TestSearchFileRegexAndResultLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.log")
	if err := os.WriteFile(path, []byte("error 100\nerror 200\ninfo 300\nerror 400\n"), 0600); err != nil { t.Fatal(err) }
	got, err := SearchFile(path, "^error [0-9]+$", true, 2)
	if err != nil { t.Fatal(err) }
	if len(got.Matches) != 2 || !got.ResultLimitReached { t.Fatalf("结果上限处理异常：%+v", got) }
}

func TestSearchFileClipsVeryLongLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "long.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", maxPreviewBytes+100)), 0600); err != nil { t.Fatal(err) }
	got, err := SearchFile(path, "xxx", false, 1)
	if err != nil { t.Fatal(err) }
	if len(got.Matches) != 1 || !got.Matches[0].ContentClipped { t.Fatalf("应当返回截断预览：%+v", got.Matches) }
}

func TestSearchFileMatchesAcrossBufferBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boundary.log")
	content := strings.Repeat("a", readerBufferSize-3) + "MAGIC\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil { t.Fatal(err) }
	got, err := SearchFile(path, "MAGIC", false, 10)
	if err != nil { t.Fatal(err) }
	if len(got.Matches) != 1 || got.Matches[0].LineNumber != 1 { t.Fatalf("跨缓冲区匹配失败：%+v", got.Matches) }
}

func TestSearchWithContextLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "context.log")
	content := "before-1\nbefore-2\nERROR target\nafter-1\nafter-2\nafter-3\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil { t.Fatal(err) }
	got, err := SearchFileWithOptions(context.Background(), path, "ERROR", false, 10, SearchOptions{ContextLines:2}, nil)
	if err != nil { t.Fatal(err) }
	if len(got.Matches) != 1 { t.Fatalf("匹配数量异常：%+v", got.Matches) }
	match := got.Matches[0]
	if len(match.ContextBefore) != 2 || match.ContextBefore[0].Content != "before-1" || match.ContextBefore[1].Content != "before-2" {
		t.Fatalf("前文异常：%+v", match.ContextBefore)
	}
	if len(match.ContextAfter) != 2 || match.ContextAfter[0].Content != "after-1" || match.ContextAfter[1].Content != "after-2" {
		t.Fatalf("后文异常：%+v", match.ContextAfter)
	}
}

func TestSearchFileCanCancelAndReportProgress(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 3*1024*1024)), 0600); err != nil { t.Fatal(err) }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reported := false
	got, err := SearchFileContext(ctx, path, "not-found", false, 10, func(p Progress) {
		if p.BytesScanned >= progressStepBytes && !reported { reported = true; cancel() }
	})
	if err != nil { t.Fatal(err) }
	if !reported || !got.Canceled { t.Fatalf("未按预期取消：reported=%v response=%+v", reported, got) }
	if got.BytesScanned >= got.FileSizeBytes { t.Fatalf("取消后不应扫描完整文件：%+v", got) }
}

func TestSearchFileEmitsMatchesBeforeScanCompletes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "incremental.log")
	content := "MATCH first line\n" + strings.Repeat("ordinary log line without the target\n", 20000)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gotMatchBeforeEnd := false
	got, err := SearchFileContext(ctx, path, "MATCH", false, 20, func(p Progress) {
		if len(p.Matches) > 0 {
			gotMatchBeforeEnd = true
			cancel()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !gotMatchBeforeEnd {
		t.Fatal("应在整个文件扫描结束前上报匹配结果")
	}
	if !got.Canceled || len(got.Matches) == 0 || got.Matches[0].Content != "MATCH first line" {
		t.Fatalf("取消后应保留已找到的增量结果：%+v", got)
	}
	if got.BytesScanned >= got.FileSizeBytes {
		t.Fatalf("增量测试应在扫描结束前取消：%+v", got)
	}
}

func TestSearchFileRejectsOversizedRegexLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "regex-long.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", maxRegexLineBytes+1)), 0600); err != nil { t.Fatal(err) }
	_, err := SearchFile(path, "x+", true, 10)
	if err == nil || !strings.Contains(err.Error(), "正则模式") { t.Fatalf("应返回超限提示，实际错误：%v", err) }
}

func TestSearchFileHandlesCRLFAndBlankLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crlf.log")
	if err := os.WriteFile(path, []byte("\r\nalpha\r\nbeta\r\n"), 0600); err != nil { t.Fatal(err) }
	got, err := SearchFile(path, "alpha", false, 10)
	if err != nil { t.Fatal(err) }
	if got.ScannedLines != 3 { t.Fatalf("扫描行数 = %d，期望 3", got.ScannedLines) }
	if len(got.Matches) != 1 || got.Matches[0].LineNumber != 2 { t.Fatalf("CRLF 处理异常：%+v", got) }
}

func TestSearchFileDetectsGB18030AndCanOverrideEncoding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gb18030.log")
	content := "INFO 用户登录成功\nERROR 数据库连接失败\n"
	encoded, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0600); err != nil {
		t.Fatal(err)
	}

	got, err := SearchFileWithOptions(context.Background(), path, "连接失败", false, 10, SearchOptions{Encoding: "auto"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Encoding != "GB18030" || len(got.Matches) != 1 || got.Matches[0].Content != "ERROR 数据库连接失败" {
		t.Fatalf("GB18030 自动识别或中文检索失败：encoding=%q matches=%+v", got.Encoding, got.Matches)
	}

	forcedUTF8, err := SearchFileWithOptions(context.Background(), path, "连接失败", false, 10, SearchOptions{Encoding: "utf-8"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if forcedUTF8.Encoding != "UTF-8" {
		t.Fatalf("手动编码选择未生效：%q", forcedUTF8.Encoding)
	}
}

func TestSearchFileDetectsUTF16BOMAndSearchesDecodedLines(t *testing.T) {
	tests := []struct {
		name string
		bom  []byte
		want string
	}{
		{name: "little-endian", bom: []byte{0xFF, 0xFE}, want: "UTF-16LE"},
		{name: "big-endian", bom: []byte{0xFE, 0xFF}, want: "UTF-16BE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "utf16.log")
			endian := textunicode.LittleEndian
			if tt.want == "UTF-16BE" {
				endian = textunicode.BigEndian
			}
			encoded, err := textunicode.UTF16(endian, textunicode.IgnoreBOM).NewEncoder().Bytes([]byte("启动服务\nERROR 请求超时\n"))
			if err != nil {
				t.Fatal(err)
			}
			data := append(append([]byte{}, tt.bom...), encoded...)
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			got, err := SearchFileWithOptions(context.Background(), path, "请求超时", false, 10, SearchOptions{Encoding: "auto"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got.Encoding != tt.want || len(got.Matches) != 1 || got.Matches[0].Content != "ERROR 请求超时" {
				t.Fatalf("UTF-16 检索失败：encoding=%q matches=%+v", got.Encoding, got.Matches)
			}
		})
	}
}

func TestSearchFileStripsUTF8BOMAndRejectsUnknownEncoding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "utf8-bom.log")
	content := append([]byte{0xEF, 0xBB, 0xBF}, []byte("INFO 服务启动\n")...)
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := SearchFileWithOptions(context.Background(), path, "服务启动", false, 10, SearchOptions{Encoding: "auto"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Encoding != "UTF-8" || len(got.Matches) != 1 || strings.HasPrefix(got.Matches[0].Content, "\uFEFF") {
		t.Fatalf("UTF-8 BOM 处理异常：encoding=%q matches=%+v", got.Encoding, got.Matches)
	}
	if _, err := SearchFileWithOptions(context.Background(), path, "INFO", false, 10, SearchOptions{Encoding: "made-up"}, nil); err == nil {
		t.Fatal("不支持的编码选项应当返回错误")
	}
}

func TestSearchFileDetectsUTF16WithoutBOMAndKeepsContext(t *testing.T) {
	tests := []struct {
		name         string
		littleEndian bool
		wantEncoding string
	}{
		{name: "little-endian", littleEndian: true, wantEncoding: "UTF-16LE"},
		{name: "big-endian", littleEndian: false, wantEncoding: "UTF-16BE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "utf16-no-bom.log")
			endian := textunicode.BigEndian
			if tt.littleEndian {
				endian = textunicode.LittleEndian
			}
			text := "INFO service ready\nERROR 请求超时\nINFO recovery\n"
			encoded, err := textunicode.UTF16(endian, textunicode.IgnoreBOM).NewEncoder().Bytes([]byte(text))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, encoded, 0600); err != nil {
				t.Fatal(err)
			}

			got, err := SearchFileWithOptions(context.Background(), path, "请求超时", false, 10, SearchOptions{
				Encoding: "auto", ContextLines: 1,
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got.Encoding != tt.wantEncoding || len(got.Matches) != 1 {
				t.Fatalf("无 BOM UTF-16 未正确识别：encoding=%q matches=%+v", got.Encoding, got.Matches)
			}
			match := got.Matches[0]
			if match.Content != "ERROR 请求超时" {
				t.Fatalf("匹配内容异常：%q", match.Content)
			}
			if len(match.ContextBefore) != 1 || match.ContextBefore[0].Content != "INFO service ready" {
				t.Fatalf("前文内容异常：%+v", match.ContextBefore)
			}
			if len(match.ContextAfter) != 1 || match.ContextAfter[0].Content != "INFO recovery" {
				t.Fatalf("后文内容异常：%+v", match.ContextAfter)
			}
		})
	}
}

func TestValidUTF8SampleOnlyAllowsIncompleteTrailingRune(t *testing.T) {
	if !validUTF8Sample([]byte("valid UTF-8 日志"), false) {
		t.Fatal("有效 UTF-8 应当被接受")
	}
	if validUTF8Sample([]byte{'v', 'a', 'l', 'i', 'd', 0xFF}, false) {
		t.Fatal("非法尾字节不能被误判为样本截断")
	}
	partialRune := append([]byte("valid UTF-8 "), 0xE4, 0xB8)
	if !validUTF8Sample(partialRune, false) {
		t.Fatal("样本末尾截断的合法多字节字符应当被接受")
	}
	if validUTF8Sample(partialRune, true) {
		t.Fatal("完整文件末尾的非法/不完整 UTF-8 不应被视为采样截断")
	}
}


func TestIncrementalSearchReportsEachMatchOnlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "incremental-deltas.log")
	line := "MATCH " + strings.Repeat("x", 80) + "\n"
	if err := os.WriteFile(path, []byte(strings.Repeat(line, 30000)), 0600); err != nil {
		t.Fatal(err)
	}

	var streamed []int64
	notifications := 0
	got, err := SearchFileContext(context.Background(), path, "MATCH", false, 5000, func(p Progress) {
		if len(p.Matches) == 0 {
			return
		}
		notifications++
		for _, match := range p.Matches {
			streamed = append(streamed, match.LineNumber)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if notifications < 2 {
		t.Fatalf("大文件应分多次发送增量结果，通知次数=%d", notifications)
	}
	if len(streamed) != len(got.Matches) {
		t.Fatalf("增量匹配数量=%d，最终结果数量=%d", len(streamed), len(got.Matches))
	}
	for i, lineNumber := range streamed {
		want := int64(i + 1)
		if lineNumber != want {
			t.Fatalf("增量结果重复或顺序错误：index=%d line=%d want=%d", i, lineNumber, want)
		}
	}
}

func TestSearchResultCountRemainsBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result-limit.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("MATCH\n", hardMaxResults+100)), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := SearchFile(path, "MATCH", false, hardMaxResults+100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Matches) != hardMaxResults || !got.ResultLimitReached {
		t.Fatalf("结果缓存应受硬上限约束：count=%d limitReached=%v", len(got.Matches), got.ResultLimitReached)
	}
}

func TestSearchFileMultiLineJSONMatchesWholeDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multiline.json")
	content := "INFO startup\n{\n  \"level\": \"error\",\n  \"message\": \"database unavailable\",\n  \"details\": {\"retry\": 3}\n}\nINFO ready\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil { t.Fatal(err) }

	got, err := SearchFileWithOptions(context.Background(), path, "database unavailable", false, 10, SearchOptions{MultiLineJSON: true}, nil)
	if err != nil { t.Fatal(err) }
	if len(got.Matches) != 1 { t.Fatalf("应匹配整个多行 JSON 文档，结果：%+v", got.Matches) }
	if got.Matches[0].LineNumber != 2 { t.Fatalf("文档起始行应为 2，实际：%d", got.Matches[0].LineNumber) }
	if !strings.Contains(got.Matches[0].Content, "\"details\"") { t.Fatalf("结果应包含完整文档预览：%s", got.Matches[0].Content) }
}

func TestSearchFileMultiLineJSONHandlesEscapedBracesAndPlainLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mixed.json")
	content := "WARN plain event\n{\n  \"message\": \"literal } and \\\"{\\\" stay in string\",\n  \"level\": \"info\"\n}\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil { t.Fatal(err) }

	got, err := SearchFileWithOptions(context.Background(), path, "WARN", false, 10, SearchOptions{MultiLineJSON: true}, nil)
	if err != nil { t.Fatal(err) }
	if len(got.Matches) != 1 || got.Matches[0].LineNumber != 1 { t.Fatalf("普通日志行也应可检索：%+v", got.Matches) }

	jsonGot, err := SearchFileWithOptions(context.Background(), path, "level", false, 10, SearchOptions{MultiLineJSON: true}, nil)
	if err != nil { t.Fatal(err) }
	if len(jsonGot.Matches) != 1 || jsonGot.Matches[0].LineNumber != 2 { t.Fatalf("JSON 字符串中的括号不应提前结束文档：%+v", jsonGot.Matches) }
}


func TestSearchFileMultiLineJSONIgnoresIncompleteDocumentAtEOF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "incomplete.json")
	content := "INFO startup\n{\n  \"message\": \"database unavailable\",\n  \"details\": {\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	got, err := SearchFileWithOptions(context.Background(), path, "database unavailable", false, 10, SearchOptions{MultiLineJSON: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Matches) != 0 {
		t.Fatalf("未闭合的 JSON 文档不应作为完整文档匹配：%+v", got.Matches)
	}
}

func TestSearchFileMultiLineJSONMatchesNestedArrayDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested-array.json")
	content := "INFO startup\n[\n  {\"message\": \"first\"},\n  [\"needle in nested array\", 2]\n]\nINFO ready\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	got, err := SearchFileWithOptions(context.Background(), path, "needle in nested array", false, 10, SearchOptions{MultiLineJSON: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Matches) != 1 || got.Matches[0].LineNumber != 2 {
		t.Fatalf("嵌套数组应作为完整文档匹配，起始行应为 2：%+v", got.Matches)
	}
	if !strings.Contains(got.Matches[0].Content, "first") || !strings.Contains(got.Matches[0].Content, "needle in nested array") {
		t.Fatalf("应返回完整文档预览：%s", got.Matches[0].Content)
	}
}

func TestSearchFileMultiLineJSONRejectsOversizedDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.json")
	var content strings.Builder
	content.WriteString("{\n")
	line := strings.Repeat("x", 64*1024)
	for content.Len() <= maxRegexLineBytes {
		content.WriteString("\"chunk\": \"")
		content.WriteString(line)
		content.WriteString("\",\n")
	}
	content.WriteString("}\n")
	if err := os.WriteFile(path, []byte(content.String()), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := SearchFileWithOptions(context.Background(), path, "needle", false, 10, SearchOptions{MultiLineJSON: true}, nil)
	if err == nil || !strings.Contains(err.Error(), "多行 JSON 文档超过 4 MiB") {
		t.Fatalf("超过 4 MiB 的 JSON 文档应返回明确错误，实际：%v", err)
	}
}


func TestSearchFileMultiLineJSONCancellationPreservesIncrementalMatches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cancel-multiline.json")
	// 让完整文档本身跨过进度通知阈值，确保回调收到匹配后能在下一轮读取前取消。
	content := "{\n  \"padding\": \"" + strings.Repeat("x", 300*1024) + "\",\n  \"message\": \"cancel-target\"\n}\n" + strings.Repeat("INFO heartbeat\\n", 40000)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelOnMatch := func(p Progress) {
		if p.MatchesFound > 0 {
			cancel()
		}
	}
	got, err := SearchFileWithOptions(ctx, path, "cancel-target", false, 10, SearchOptions{MultiLineJSON: true}, cancelOnMatch)
	if err != nil {
		t.Fatalf("取消检索不应作为错误返回：%v", err)
	}
	if !got.Canceled {
		t.Fatal("触发取消后应标记 Canceled")
	}
	if len(got.Matches) != 1 || !strings.Contains(got.Matches[0].Content, "cancel-target") {
		t.Fatalf("取消检索应保留已找到的增量结果：%+v", got.Matches)
	}
}
