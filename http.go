package raven

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
)

func NewHttp(req *http.Request) *Http {
	proto := "http"
	if req.TLS != nil || req.Header.Get("X-Forwarded-Proto") == "https" {
		proto = "https"
	}
	h := &Http{
		Method:  req.Method,
		Cookies: req.Header.Get("Cookie"),
		Query:   url.Values(sanitizeValues(req.URL.Query())).Encode(),
		URL:     proto + "://" + req.Host + req.URL.Path,
		Headers: make(map[string]string, len(req.Header)),
	}
	if addr, port, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		h.Env = map[string]string{"REMOTE_ADDR": addr, "REMOTE_PORT": port}
	}

	for k, v := range http.Header(sanitizeValues(req.Header)) {
		h.Headers[k] = strings.Join(v, ",")
	}
	return h
}

var querySecretFields = []string{"password", "passphrase", "passwd", "secret"}

func sanitizeValues(query map[string][]string) map[string][]string {
	for _, keyword := range querySecretFields {
		for field := range query {
			if strings.Contains(strings.ToLower(field), strings.ToLower(keyword)) {
				query[field] = []string{"********"}
			}
		}
	}
	return query
}

// AddSanitizewField adds a custom sanitize field to the array of fields to
// search for and sanitize. This allows you to hide sensitive information in
// both the query string and headers.
func AddSanitizeField(field string) {
	querySecretFields = append(querySecretFields, field)
}

// https://docs.getsentry.com/hosted/clientdev/interfaces/#context-interfaces
type Http struct {
	// Required
	URL    string `json:"url"`
	Method string `json:"method"`
	Query  string `json:"query_string,omitempty"`

	// Optional
	Cookies string            `json:"cookies,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Env     map[string]string `json:"env,omitempty"`

	// Must be either a string or map[string]string
	Data interface{} `json:"data,omitempty"`
}

func (h *Http) Class() string { return "request" }

// Recovery handler to wrap the stdlib net/http Mux. This function will detect a
// panic, report it, and recover from the panic, preventing it from continuing
// further.
//
// Example:
//	http.HandleFunc("/", raven.ReportHandler(func(w http.ResponseWriter, r *http.Request) {
//		...
//	}))
func RecoveryHandler(handler func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rval := recover(); rval != nil {
				debug.PrintStack()
				rvalStr := fmt.Sprint(rval)
				packet := NewPacket(rvalStr, NewException(errors.New(rvalStr), NewStacktrace(2, 3, nil)), NewHttp(r))
				Capture(packet, nil)
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()

		handler(w, r)
	}
}

// Report handler to wrap the stdlib net/http Mux. This function will detect a
// panic, report it, and allow the panic to contune.
//
// Example:
//	http.HandleFunc("/", raven.ReportHandler(func(w http.ResponseWriter, r *http.Request) {
//		...
//	}))
func ReportHandler(handler func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rval := recover(); rval != nil {
				debug.PrintStack()
				rvalStr := fmt.Sprint(rval)
				packet := NewPacket(rvalStr, NewException(errors.New(rvalStr), NewStacktrace(2, 3, nil)), NewHttp(r))
				Capture(packet, nil)
				w.WriteHeader(http.StatusInternalServerError)
				panic(rval)
			}
		}()

		handler(w, r)
	}
}
