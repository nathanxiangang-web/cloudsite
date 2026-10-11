package p115

import (
	"bytes"
	"encoding/json"
	"github.com/SheltonZhu/115driver/pkg/driver"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

// startHTTPTrace retains only closed metadata. Response bytes are transient,
// bounded, and cleared after classification; requests are never recorded.
type startHTTPTrace struct {
	status                             atomic.Int32
	mu                                 sync.Mutex
	operation, shape                   string
	outerValid, observed               bool
	outerParsed                        bool
	mediaFamily, sizeBucket, jsonValid string
}

func (trace *startHTTPTrace) reset() {
	trace.status.Store(0)
	trace.mu.Lock()
	defer trace.mu.Unlock()
	trace.operation, trace.shape = "UNKNOWN", "UNKNOWN"
	trace.outerValid, trace.observed = false, false
	trace.outerParsed = false
	trace.mediaFamily, trace.sizeBucket, trace.jsonValid = "UNKNOWN", "UNKNOWN", "UNKNOWN"
}

// Only closed metadata survives the request. No body, URL or header is kept.
type tracedBody struct {
	io.ReadCloser
	trace       *startHTTPTrace
	buffer      []byte
	oversize    bool
	finished    bool
	mediaFamily string
}

const startResponseLimit = 64 * 1024

func (body *tracedBody) Read(p []byte) (int, error) {
	n, err := body.ReadCloser.Read(p)
	if !body.oversize {
		if len(body.buffer)+n > startResponseLimit {
			body.oversize = true
		} else {
			body.buffer = append(body.buffer, p[:n]...)
		}
	}
	if err == io.EOF {
		body.finish(true)
	}
	return n, err
}

func (body *tracedBody) Close() error {
	body.finish(false)
	return body.ReadCloser.Close()
}

func (body *tracedBody) finish(complete bool) {
	if body.finished {
		return
	}
	body.finished = true
	body.trace.mu.Lock()
	defer body.trace.mu.Unlock()
	if body.trace.operation == "OFFLINE_POST" {
		body.trace.mediaFamily = body.mediaFamily
		body.trace.sizeBucket, body.trace.jsonValid = "INCOMPLETE", "UNKNOWN"
		if complete {
			body.trace.sizeBucket = "OVERSIZE"
			if !body.oversize {
				body.trace.sizeBucket = responseSizeBucket(len(body.buffer))
				body.trace.jsonValid = "FALSE"
				if json.Valid(body.buffer) {
					body.trace.jsonValid = "TRUE"
				}
			}
		}
	}
	if complete && !body.oversize {
		trimmed := bytes.TrimSpace(body.buffer)
		shape := "TEXT"
		if len(trimmed) == 0 {
			shape = "EMPTY"
		} else {
			switch trimmed[0] {
			case '{':
				shape = "OBJECT"
			case '[':
				shape = "ARRAY"
			case '<':
				shape = "HTML"
			}
		}
		body.trace.shape, body.trace.outerValid, body.trace.observed = shape, json.Valid(body.buffer), true
		if body.trace.operation == "OFFLINE_POST" {
			var envelope driver.DownloadResp
			body.trace.outerParsed = json.Unmarshal(body.buffer, &envelope) == nil
		}
	}
	for i := range body.buffer {
		body.buffer[i] = 0
	}
	body.buffer = nil
}

type statusTransport struct {
	next  http.RoundTripper
	trace *startHTTPTrace
}

func (transport statusTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.trace.reset()
	transport.trace.mu.Lock()
	switch {
	case request.Method == http.MethodGet && request.URL.Scheme == "https" && request.URL.Host == "my.115.com" && request.URL.Path == "/" && request.URL.Query().Get("ct") == "ajax" && request.URL.Query().Get("ac") == "nav":
		transport.trace.operation = "USER_INFO_GET"
	default:
		// The SDK adds a timestamp query to this exact endpoint.
		if request.Method == http.MethodPost && request.URL.Scheme == "https" && request.URL.Host == "lixian.115.com" && request.URL.Path == "/lixianssp/" && request.URL.Query().Get("ac") == "add_task_urls" {
			transport.trace.operation = "OFFLINE_POST"
		}
	}
	transport.trace.mu.Unlock()
	response, err := transport.next.RoundTrip(request)
	if response != nil {
		transport.trace.status.Store(int32(response.StatusCode))
		if response.Body != nil {
			response.Body = &tracedBody{ReadCloser: response.Body, trace: transport.trace, mediaFamily: responseMediaFamily(response.Header.Get("Content-Type"))}
		}
	}
	return response, err
}

// Header values never survive this finite classification. SDK ForceContentType
// controls parsing, not the server's actual media type.
func responseMediaFamily(value string) string {
	if strings.TrimSpace(value) == "" {
		return "MISSING"
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return "OTHER"
	}
	switch strings.ToLower(mediaType) {
	case "application/json", "application/problem+json":
		return "JSON"
	case "text/plain":
		return "TEXT"
	case "text/html", "application/xhtml+xml":
		return "HTML"
	default:
		return "OTHER"
	}
}

// Exclusive byte ranges; larger/incomplete reads have separate sentinels.
func responseSizeBucket(size int) string {
	switch {
	case size == 0:
		return "0"
	case size < 128:
		return "1-127"
	case size < 1024:
		return "128-1023"
	case size < 4096:
		return "1024-4095"
	default:
		return "4K-64K"
	}
}
