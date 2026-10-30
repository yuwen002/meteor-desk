// Package httpx provides bounded local HTTP requests for the desktop toolkit.
package httpx

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxRequestBodyBytes  = 2 * 1024 * 1024
	maxResponseBodyBytes = 2 * 1024 * 1024
	// defaultRequestTimeoutMs 是未指定超时时的默认值；maxRequestTimeoutMs 限制用户可配置范围。
	defaultRequestTimeoutMs = 30000
	maxRequestTimeoutMs     = 120000
	// maxRedirects 限制自动跟随的重定向次数，避免重定向循环。
	maxRedirects = 10
)

type Request struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	// TimeoutMs 是单次请求的超时毫秒数；0 使用默认 30 秒，上限 120 秒。
	TimeoutMs int `json:"timeoutMs,omitempty"`
	// FollowRedirects 为 false 时直接返回 3xx 响应；nil 保持默认跟随（最多 10 次）。
	FollowRedirects *bool `json:"followRedirects,omitempty"`
	// InsecureSkipVerify 跳过 TLS 证书校验，仅用于本机自签证书调试。
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

type Response struct {
	Status        int               `json:"status"`
	StatusText    string            `json:"statusText"`
	Headers       map[string]string `json:"headers"`
	Body          string            `json:"body"`
	BodyTruncated bool              `json:"bodyTruncated"`
	SizeBytes     int               `json:"sizeBytes"`
	DurationMs    int64             `json:"durationMs"`
	// FinalURL 是跟随重定向后的最终地址；未跟随重定向时等于请求地址。
	FinalURL string `json:"finalUrl,omitempty"`
	// Redirects 是实际跟随的重定向次数。
	Redirects int `json:"redirects"`
	// Binary 表示响应正文包含 NUL 或非法 UTF-8，界面只能展示带替换字符的预览。
	Binary bool `json:"binary"`
}

func Send(request Request) (Response, error) {
	method := strings.ToUpper(strings.TrimSpace(request.Method))
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
	default:
		return Response{}, errors.New("不支持的 HTTP 方法")
	}
	endpoint, err := url.ParseRequestURI(strings.TrimSpace(request.URL))
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return Response{}, errors.New("请输入有效的 http 或 https URL")
	}
	if len(request.Body) > maxRequestBodyBytes {
		return Response{}, fmt.Errorf("请求正文超过 %d MiB 限制", maxRequestBodyBytes/(1024*1024))
	}
	if len(request.Headers) > 100 {
		return Response{}, errors.New("请求头数量不能超过 100")
	}
	timeoutMs := request.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = defaultRequestTimeoutMs
	}
	if timeoutMs > maxRequestTimeoutMs {
		return Response{}, fmt.Errorf("请求超时不能超过 %d 秒", maxRequestTimeoutMs/1000)
	}

	httpRequest, err := http.NewRequest(method, endpoint.String(), bytes.NewBufferString(request.Body))
	if err != nil {
		return Response{}, fmt.Errorf("创建请求失败：%w", err)
	}
	for key, value := range request.Headers {
		if strings.TrimSpace(key) == "" {
			return Response{}, errors.New("请求头名称不能为空")
		}
		httpRequest.Header.Set(key, value)
	}
	startedAt := time.Now()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if request.InsecureSkipVerify {
		// 仅响应用户显式开启的调试选项；默认仍校验证书。
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	client := &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond, Transport: transport}
	redirects := 0
	if request.FollowRedirects != nil && !*request.FollowRedirects {
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	} else {
		client.CheckRedirect = func(_ *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("重定向次数超过 %d 次", maxRedirects)
			}
			redirects = len(via)
			return nil
		}
	}
	httpResponse, err := client.Do(httpRequest)
	if err != nil {
		var netErr net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
			return Response{}, fmt.Errorf("请求超时（%d ms）", timeoutMs)
		}
		return Response{}, fmt.Errorf("请求失败：%w", err)
	}
	defer httpResponse.Body.Close()
	body, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponseBodyBytes+1))
	if err != nil {
		return Response{}, fmt.Errorf("读取响应失败：%w", err)
	}
	received := len(body)
	truncated := received > maxResponseBodyBytes
	if truncated {
		body = body[:maxResponseBodyBytes]
	}
	headers := make(map[string]string, len(httpResponse.Header))
	for key, values := range httpResponse.Header {
		headers[key] = strings.Join(values, ", ")
	}
	finalURL := endpoint.String()
	if httpResponse.Request != nil && httpResponse.Request.URL != nil {
		finalURL = httpResponse.Request.URL.String()
	}
	return Response{Status: httpResponse.StatusCode, StatusText: http.StatusText(httpResponse.StatusCode), Headers: headers, Body: strings.ToValidUTF8(string(body), "�"), BodyTruncated: truncated, SizeBytes: received, DurationMs: time.Since(startedAt).Milliseconds(), FinalURL: finalURL, Redirects: redirects, Binary: looksBinary(body)}, nil
}

// looksBinary 判断响应是否包含 NUL 或非法 UTF-8，用于提示界面只展示文本预览。
func looksBinary(body []byte) bool {
	if bytes.IndexByte(body, 0) >= 0 {
		return true
	}
	return !utf8.Valid(body)
}
