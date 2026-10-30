// Package httpx provides bounded local HTTP requests for the desktop toolkit.
package httpx

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxRequestBodyBytes  = 2 * 1024 * 1024
	maxResponseBodyBytes = 2 * 1024 * 1024
)

type Request struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type Response struct {
	Status        int               `json:"status"`
	StatusText    string            `json:"statusText"`
	Headers       map[string]string `json:"headers"`
	Body          string            `json:"body"`
	BodyTruncated bool              `json:"bodyTruncated"`
	SizeBytes     int               `json:"sizeBytes"`
	DurationMs    int64             `json:"durationMs"`
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
	client := &http.Client{Timeout: 30 * time.Second}
	httpResponse, err := client.Do(httpRequest)
	if err != nil {
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
	return Response{Status: httpResponse.StatusCode, StatusText: http.StatusText(httpResponse.StatusCode), Headers: headers, Body: strings.ToValidUTF8(string(body), "�"), BodyTruncated: truncated, SizeBytes: received, DurationMs: time.Since(startedAt).Milliseconds()}, nil
}
