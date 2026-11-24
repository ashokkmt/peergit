package campus

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/resend/resend-go/v4"
	"peergit/internal/platform/jobs"
)

func TestHTMLEscape(t *testing.T) {
	got := htmlEscape(`<tag a="b">&'`)
	if got != "&lt;tag a=&quot;b&quot;&gt;&amp;&#39;" {
		t.Fatalf("htmlEscape() = %q", got)
	}
}

func TestSendResendClassifiesPermanentClientErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"name":"validation_error","message":"invalid request"}`))
	}))
	defer server.Close()
	client := resend.NewClient("re_test")
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client.BaseURL = baseURL
	_, err = sendResendWithClient(context.Background(), client, "delivery-123", deliveryPayload{To: "test@example.org", Code: "123456"})
	var permanent *jobs.PermanentError
	if !errors.As(err, &permanent) || permanent.Code != "resend_rejected" {
		t.Fatalf("permanent error = %#v, want resend_rejected", err)
	}
}

func TestSendResendClassifiesTransientAndConflictingResponses(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		retryAfter string
		permanent  string
		minDelay   time.Duration
	}{
		{name: "malformed success retries", status: http.StatusOK, body: `not-json`},
		{name: "server error retries", status: http.StatusServiceUnavailable, body: `{"name":"internal_error","message":"try again"}`},
		{name: "concurrent idempotent request retries", status: http.StatusConflict, body: `{"name":"concurrent_idempotent_requests","message":"request in progress"}`},
		{name: "idempotency payload conflict stops", status: http.StatusConflict, body: `{"name":"invalid_idempotent_request","message":"payload differs"}`, permanent: "resend_idempotency_conflict"},
		{name: "rate limit honors retry after", status: http.StatusTooManyRequests, body: `{"name":"rate_limit_exceeded","message":"slow down"}`, retryAfter: "17", minDelay: 17 * time.Second},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if test.retryAfter != "" {
					w.Header().Set("Retry-After", test.retryAfter)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			client := resend.NewClient("re_test")
			baseURL, err := url.Parse(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			client.BaseURL = baseURL
			_, err = sendResendWithClient(context.Background(), client, "delivery-123", deliveryPayload{To: "test@example.org"})
			var permanent *jobs.PermanentError
			if test.permanent != "" {
				if !errors.As(err, &permanent) || permanent.Code != test.permanent {
					t.Fatalf("error = %#v, want permanent code %q", err, test.permanent)
				}
				return
			}
			if err == nil || errors.As(err, &permanent) {
				t.Fatalf("error = %#v, want retryable error", err)
			}
			if test.minDelay > 0 {
				var retryAfter interface{ RetryAfterDuration(time.Time) time.Duration }
				if !errors.As(err, &retryAfter) || retryAfter.RetryAfterDuration(time.Now()) < test.minDelay {
					t.Fatalf("error does not retain Retry-After lower bound: %#v", err)
				}
			}
		})
	}
}

func TestSendResendUsesStableIdempotencyAndSafeTemplate(t *testing.T) {
	var gotKey, gotAuth string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Idempotency-Key")
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"email_123"}`))
	}))
	defer server.Close()
	client := resend.NewClient("re_test")
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client.BaseURL = baseURL
	id, err := sendResendWithClient(context.Background(), client, "delivery-123", deliveryPayload{To: "test@example.org", Link: "https://example.org/?x=<&", Code: "123456"})
	if err != nil || id != "email_123" {
		t.Fatalf("sendResendWithClient() = %q, %v", id, err)
	}
	if gotKey != "campus-verification/delivery-123" || gotAuth != "Bearer re_test" {
		t.Fatalf("idempotency/auth headers = %q / %q", gotKey, gotAuth)
	}
	if body["reply_to"] != "support@peergit.com" || body["from"] != "PeerGit Accounts <auth@mail.peergit.com>" {
		t.Fatalf("sender fields = %#v", body)
	}
	if body["html"] == nil || body["text"] == nil {
		t.Fatal("expected both HTML and plain-text email bodies")
	}
}

func TestResendSenderReusesConfiguredHTTPClient(t *testing.T) {
	var requests, connections atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"email_123"}`))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.Start()
	defer server.Close()

	sender := NewResendSender("re_test")
	defer sender.CloseIdleConnections()
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	sender.client.BaseURL = baseURL
	if sender.httpClient.Timeout != resendRequestTimeout {
		t.Fatalf("HTTP timeout = %s, want %s", sender.httpClient.Timeout, resendRequestTimeout)
	}
	for range 2 {
		if _, err := sender.send(context.Background(), "delivery-123", deliveryPayload{To: "test@example.org"}); err != nil {
			t.Fatalf("send through shared client: %v", err)
		}
	}
	if requests.Load() != 2 || connections.Load() != 1 {
		t.Fatalf("requests=%d connections=%d, want 2 requests over 1 reused connection", requests.Load(), connections.Load())
	}
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := sender.send(context.Background(), "delivery-123", deliveryPayload{To: "test@example.org"})
			errs <- err
		}()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent send through shared client: %v", err)
		}
	}
	if requests.Load() != 4 {
		t.Fatalf("requests=%d, want 4 total", requests.Load())
	}
}
