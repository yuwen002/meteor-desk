package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func boolPointer(value bool) *bool { return &value }

func TestSendSendsHeadersAndBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("X-Trace") != "trace-1" {
			t.Error("request metadata was not preserved")
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	response, err := Send(Request{Method: "post", URL: server.URL, Headers: map[string]string{"X-Trace": "trace-1"}, Body: `{"name":"meteor"}`})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusCreated || response.StatusText != http.StatusText(http.StatusCreated) || response.Body != `{"ok":true}` || response.Headers["Content-Type"] != "application/json" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestSendRejectsInvalidRequestsAndTruncatesLargeResponse(t *testing.T) {
	if _, err := Send(Request{Method: "TRACE", URL: "https://example.com"}); err == nil {
		t.Fatal("unsupported method should fail")
	}
	if _, err := Send(Request{Method: "GET", URL: "file:///tmp/example"}); err == nil {
		t.Fatal("unsupported scheme should fail")
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(strings.Repeat("x", maxResponseBodyBytes+1)))
	}))
	defer server.Close()
	response, err := Send(Request{Method: "GET", URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if !response.BodyTruncated || len(response.Body) != maxResponseBodyBytes || response.SizeBytes != maxResponseBodyBytes+1 {
		t.Fatalf("response should be bounded: %+v", response)
	}
}

func TestSendFollowsRedirectsByDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/start" {
			http.Redirect(writer, request, "/final", http.StatusFound)
			return
		}
		_, _ = writer.Write([]byte("final body"))
	}))
	defer server.Close()
	response, err := Send(Request{Method: "GET", URL: server.URL + "/start"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusOK || response.Redirects != 1 || response.FinalURL != server.URL+"/final" || response.Body != "final body" {
		t.Fatalf("unexpected redirect response: %+v", response)
	}
}

func TestSendReturnsRedirectWhenFollowingIsDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/start" {
			http.Redirect(writer, request, "/final", http.StatusFound)
			return
		}
		_, _ = writer.Write([]byte("final body"))
	}))
	defer server.Close()
	response, err := Send(Request{Method: "GET", URL: server.URL + "/start", FollowRedirects: boolPointer(false)})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusFound || response.Redirects != 0 || response.Body == "final body" || response.FinalURL != server.URL+"/start" {
		t.Fatalf("redirect should not be followed: %+v", response)
	}
}

func TestSendRejectsExcessiveTimeout(t *testing.T) {
	_, err := Send(Request{Method: "GET", URL: "https://example.com", TimeoutMs: maxRequestTimeoutMs + 1})
	if err == nil || !strings.Contains(err.Error(), "请求超时不能超过") {
		t.Fatalf("expected timeout validation error, got %v", err)
	}
}

func TestSendHonorsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = writer.Write([]byte("late"))
	}))
	defer server.Close()
	_, err := Send(Request{Method: "GET", URL: server.URL, TimeoutMs: 50})
	if err == nil || !strings.Contains(err.Error(), "请求超时") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestSendSkipsTLSCertificateVerificationWhenRequested(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("secure"))
	}))
	defer server.Close()
	if _, err := Send(Request{Method: "GET", URL: server.URL}); err == nil {
		t.Fatal("self-signed certificate should fail by default")
	}
	response, err := Send(Request{Method: "GET", URL: server.URL, InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusOK || response.Body != "secure" {
		t.Fatalf("unexpected TLS response: %+v", response)
	}
}

func TestSendFlagsBinaryResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte{0x00, 0x01, 0xff, 0xfe})
	}))
	defer server.Close()
	response, err := Send(Request{Method: "GET", URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if !response.Binary {
		t.Fatalf("binary payload should be flagged: %+v", response)
	}
}
