package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
