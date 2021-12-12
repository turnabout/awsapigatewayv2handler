package awsapigatewayv2handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/google/go-cmp/cmp"
)

func TestLambdaEventToHTTPRequest(t *testing.T) {
	tests := []struct {
		name     string
		event    events.APIGatewayV2HTTPRequest
		expected func() *http.Request
	}{
		{
			name: "basic GET",
			event: events.APIGatewayV2HTTPRequest{
				RawPath: "/path",
			},
			expected: func() *http.Request {
				r, err := http.NewRequest(http.MethodGet, "/path", nil)
				if err != nil {
					panic(err)
				}
				return r
			},
		},
		{
			name: "basic POST",
			event: events.APIGatewayV2HTTPRequest{
				RawPath: "/path",
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: "POST",
					},
				},
			},
			expected: func() *http.Request {
				r, err := http.NewRequest(http.MethodPost, "/path", nil)
				if err != nil {
					panic(err)
				}
				return r
			},
		},
		{
			name: "headers",
			event: events.APIGatewayV2HTTPRequest{
				RawPath: "/path",
				Headers: map[string]string{
					"Accept":     "*",
					"User-Agent": "Chrome, or something",
				},
			},
			expected: func() *http.Request {
				r, err := http.NewRequest(http.MethodGet, "/path", nil)
				if err != nil {
					panic(err)
				}
				r.Header.Add("Accept", "*")
				r.Header.Add("User-Agent", "Chrome, or something")
				return r
			},
		},
		{
			name: "querystring",
			event: events.APIGatewayV2HTTPRequest{
				RawPath:        "/path",
				RawQueryString: "a=123&b=456",
			},
			expected: func() *http.Request {
				r, err := http.NewRequest(http.MethodGet, "/path?a=123&b=456", nil)
				if err != nil {
					panic(err)
				}
				return r
			},
		},
		{
			name: "JSON POST",
			event: events.APIGatewayV2HTTPRequest{
				RawPath: "/path",
				Body:    "{}",
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				RequestContext: events.APIGatewayV2HTTPRequestContext{

					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: "POST",
						Path:   "/path",
					},
				},
			},
			expected: func() *http.Request {
				r, err := http.NewRequest(http.MethodPost, "/path", strings.NewReader("{}"))
				if err != nil {
					panic(err)
				}
				r.Header.Add("Content-Type", "application/json")
				return r
			},
		},
		{
			name: "base64 encoded body",
			event: events.APIGatewayV2HTTPRequest{
				RawPath:         "/path",
				Body:            base64.StdEncoding.EncodeToString([]byte("12345")),
				IsBase64Encoded: true,
				RequestContext: events.APIGatewayV2HTTPRequestContext{

					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: "POST",
						Path:   "/path",
					},
				},
			},
			expected: func() *http.Request {
				r, err := http.NewRequest(http.MethodPost, "/path", strings.NewReader("12345"))
				if err != nil {
					panic(err)
				}
				return r
			},
		},
		{
			name: "cookies",
			event: events.APIGatewayV2HTTPRequest{
				RawPath: "/path",
				Body:    "",
				Headers: map[string]string{
					"Cookie": "name=value; name2=value2; name3=value3",
				},
			},
			expected: func() *http.Request {
				r, err := http.NewRequest(http.MethodGet, "/path", nil)
				if err != nil {
					panic(err)
				}
				r.AddCookie(&http.Cookie{Name: "name", Value: "value"})
				r.AddCookie(&http.Cookie{Name: "name2", Value: "value2"})
				r.AddCookie(&http.Cookie{Name: "name3", Value: "value3"})
				return r
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange.
			expected := test.expected()

			// Act.
			actual, err := convertLambdaEventToHTTPRequest(test.event)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Assert.
			compare(expected.Body, actual.Body, t)
			if expected.ContentLength != actual.ContentLength {
				t.Errorf("content length: expected %d, got %d", expected.ContentLength, actual.ContentLength)
			}
			if diff := cmp.Diff(expected.Form, actual.Form); diff != "" {
				t.Errorf("form:\n%s", diff)
			}
			if diff := cmp.Diff(expected.Header, actual.Header); diff != "" {
				t.Errorf("header:\n%s", diff)
			}
			if expected.Host != actual.Host {
				t.Errorf("expected method %q, got %q", expected.Host, actual.Host)
			}
			if expected.Method != actual.Method {
				t.Errorf("expected method %q, got %q", expected.Method, actual.Method)
			}
			if expected.URL.String() != actual.URL.String() {
				t.Errorf("expected method %q, got %q", expected.URL.String(), actual.URL.String())
			}
		})
	}
}

func compare(expected, actual io.Reader, t *testing.T) {
	if expected == nil && actual != nil {
		t.Errorf("body: expected nil, but wasn't")
		return
	}
	if expected != nil && actual == nil {
		t.Errorf("body: expected non-nil, but was nil")
		return
	}
	if expected == nil && actual == nil {
		return
	}
	bytesExpected, errExpected := ioutil.ReadAll(expected)
	if errExpected != nil {
		t.Errorf("body: error reading from expected: %v", errExpected)
		return
	}
	bytesActual, errActual := ioutil.ReadAll(actual)
	if errActual != nil {
		t.Errorf("body: error reading from actual: %v", errActual)
		return
	}
	if diff := cmp.Diff(bytesExpected, bytesActual); diff != "" {
		t.Errorf("body:\n%v", diff)
	}
}

func TestHTTPResponseToLambdaEvent(t *testing.T) {
	tests := []struct {
		name     string
		resp     *http.Response
		expected events.APIGatewayV2HTTPResponse
	}{
		{
			name: "headers",
			resp: &http.Response{
				Status:     "OK",
				StatusCode: 200,
				Header: map[string][]string{
					"X-Powered-By": {"Annoying server that includes this."},
				},
				Body:          io.NopCloser(strings.NewReader("Hello, World")),
				ContentLength: int64(len("Hello, World")),
				Trailer: map[string][]string{
					"Trailing": {"TrailingValue"},
				},
			},
			expected: events.APIGatewayV2HTTPResponse{
				StatusCode: 200,
				MultiValueHeaders: map[string][]string{
					"Content-Length": {strconv.Itoa(len("Hello, World"))},
					"X-Powered-By":   {"Annoying server that includes this."},
					"Trailing":       {"TrailingValue"},
				},
				Body:            "Hello, World",
				IsBase64Encoded: false,
			},
		},
		{
			name: "binary content",
			resp: &http.Response{
				Status:     "OK",
				StatusCode: 200,
				Header: map[string][]string{
					"Content-Type": {"application/pdf"},
				},
				Body:          io.NopCloser(strings.NewReader("Not really a PDF")),
				ContentLength: int64(len("Not really a PDF")),
			},
			expected: events.APIGatewayV2HTTPResponse{
				StatusCode: 200,
				MultiValueHeaders: map[string][]string{
					"Content-Length": {strconv.Itoa(len("Not really a PDF"))},
					"Content-Type":   {"application/pdf"},
				},
				Body:            base64.StdEncoding.EncodeToString([]byte("Not really a PDF")),
				IsBase64Encoded: true,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange.
			expected := test.expected

			// Act.
			actual, err := convertHTTPResponseToLambdaEvent(test.resp)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Assert.
			if diff := cmp.Diff(expected, actual); diff != "" {
				t.Errorf("response:\n%s", diff)
			}
		})
	}
}

func TestHTTPHandlers(t *testing.T) {
	tests := []struct {
		name    string
		handler http.Handler
		req     events.APIGatewayV2HTTPRequest
		resp    events.APIGatewayV2HTTPResponse
	}{
		{
			name: "Hello, World",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, "Hello, World")
			}),
			req: events.APIGatewayV2HTTPRequest{
				RawPath: "/path",
			},
			resp: events.APIGatewayV2HTTPResponse{
				StatusCode:      200,
				Body:            "Hello, World",
				IsBase64Encoded: false,
				MultiValueHeaders: map[string][]string{
					"Content-Type": {"text/plain; charset=utf-8"},
				},
			},
		},
		{
			name: "JSON response",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				name := r.URL.Query().Get("name")
				json.NewEncoder(w).Encode(struct {
					Message string `json:"msg"`
				}{
					Message: "Hello " + name,
				})
			}),
			req: events.APIGatewayV2HTTPRequest{
				RawPath:        "/path",
				RawQueryString: "name=Adrian",
			},
			resp: events.APIGatewayV2HTTPResponse{
				StatusCode:      200,
				Body:            `{"msg":"Hello Adrian"}` + "\n",
				IsBase64Encoded: false,
				MultiValueHeaders: map[string][]string{
					"Content-Type": {"application/json"},
				},
			},
		},
		{

			name: "Set status",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}),
			req: events.APIGatewayV2HTTPRequest{
				RawPath: "/path",
			},
			resp: events.APIGatewayV2HTTPResponse{
				StatusCode:        404,
				Body:              "",
				IsBase64Encoded:   false,
				MultiValueHeaders: map[string][]string{},
			},
		},
		{

			name: "Set headers",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Add("X-Custom", "thing")
				w.Header().Add("X-Custom-2", "don't need the X- anymore")
			}),
			req: events.APIGatewayV2HTTPRequest{
				RawPath: "/path",
			},
			resp: events.APIGatewayV2HTTPResponse{
				StatusCode:      200,
				Body:            "",
				IsBase64Encoded: false,
				MultiValueHeaders: map[string][]string{
					"X-Custom":   {"thing"},
					"X-Custom-2": {"don't need the X- anymore"},
				},
			},
		},
		{

			name: "Set cookies",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.SetCookie(w, &http.Cookie{
					Name:  "cookie1",
					Value: "value1",
				})
				http.SetCookie(w, &http.Cookie{
					Name:  "cookie2",
					Value: "value2",
				})
				io.WriteString(w, "Hello, World")
			}),
			req: events.APIGatewayV2HTTPRequest{
				RawPath: "/path",
			},
			resp: events.APIGatewayV2HTTPResponse{
				StatusCode: 200,
				MultiValueHeaders: map[string][]string{
					"Content-Type": {"text/plain; charset=utf-8"},
					"Set-Cookie": {
						"cookie1=value1",
						"cookie2=value2",
					},
				},
				Body:            "Hello, World",
				IsBase64Encoded: false,
				Cookies:         []string{"cookie1=value1", "cookie2=value2"},
			},
		},
		{

			name: "Binary content",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "image/jpeg")
				io.Copy(w, strings.NewReader("test"))
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %v", r.Method)
				}
			}),
			req: events.APIGatewayV2HTTPRequest{
				RawPath: "/path",
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: "POST",
					},
				},
			},
			resp: events.APIGatewayV2HTTPResponse{
				StatusCode: 200,
				MultiValueHeaders: map[string][]string{
					"Content-Type": {"image/jpeg"},
				},
				Body:            base64.StdEncoding.EncodeToString([]byte("test")),
				IsBase64Encoded: true,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange.
			lh := LambdaHandler{
				Handler: test.handler,
			}
			payload, err := json.Marshal(test.req)
			expected := test.resp

			// Act.
			responseBytes, err := lh.Invoke(context.Background(), payload)
			if err != nil {
				t.Fatalf("error executing request: %v", err)
			}
			var actual events.APIGatewayV2HTTPResponse
			err = json.Unmarshal(responseBytes, &actual)
			if err != nil {
				t.Fatalf("error unmarshalling response: %v", err)
			}

			// Assert.
			if diff := cmp.Diff(expected, actual); diff != "" {
				t.Errorf("response:\n%s", diff)
			}
		})
	}
}