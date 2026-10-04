package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"
)

func TestHTTPServerHasHeaderAndIdleTimeouts(t *testing.T) {
	server := newHTTPServer(http.NotFoundHandler())
	if server.ReadHeaderTimeout <= 0 || server.ReadTimeout <= 0 || server.IdleTimeout <= 0 || server.MaxHeaderBytes <= 0 {
		t.Fatalf("server protections are not configured: %#v", server)
	}
}

func TestRunInstallsConfiguredLoggerBeforeListenFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	t.Setenv("APP_ENV", "production")
	t.Setenv("LOG_LEVEL", "error")
	t.Setenv("HTTP_ADDR", listener.Addr().String())
	t.Setenv("DATABASE_URL", "postgres://peergit:local-development-only@127.0.0.1:5432/peergit?sslmode=disable")
	t.Setenv("DATABASE_MAX_CONNS", "1")
	t.Setenv("CURSOR_SIGNING_KEY", "test-only-production-cursor-key-0123456789")
	t.Setenv("APP_ORIGIN", "https://example.edu")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("SESSION_HASH_KEY", "test-only-session-hash-key-0123456789")
	t.Setenv("MFA_ENCRYPTION_KEY", "test-only-mfa-encryption-key-0123456789")
	t.Setenv("GITHUB_CLIENT_ID", "test-client-id")
	t.Setenv("GITHUB_CLIENT_SECRET", "test-client-secret")
	t.Setenv("GITHUB_REDIRECT_URL", "https://example.edu/api/v1/auth/github/callback")
	t.Setenv("SMTP_HOST", "smtp.example.edu:587")
	t.Setenv("SMTP_FROM", "PeerGit <notify@example.edu>")
	t.Setenv("SMTP_TLS_MODE", "starttls")
	t.Setenv("CAMPUS_VERIFICATION_HASH_KEY", "test-only-verification-hash-key-0123456789")
	t.Setenv("VERIFICATION_EMAIL_ENCRYPTION_KEY", "test-only-verification-email-key-0123456789")
	previousLogger, previousStdout := slog.Default(), os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	defer func() {
		os.Stdout = previousStdout
		slog.SetDefault(previousLogger)
	}()
	os.Stdout = writer
	err = run()
	if err == nil {
		t.Fatal("expected occupied address to fail startup")
	}
	slog.Error("API server stopped", "error", err)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	var record struct {
		Level   string `json:"level"`
		Message string `json:"msg"`
		Error   string `json:"error"`
	}
	if err := json.NewDecoder(reader).Decode(&record); err != nil {
		t.Fatalf("startup failure did not use configured JSON logger: %v", err)
	}
	if record.Level != "ERROR" || record.Message != "API server stopped" || record.Error == "" {
		t.Fatalf("unexpected startup log: %#v", record)
	}
}

func TestServeDrainsActiveRequestOnCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, release, shuttingDown := make(chan struct{}), make(chan struct{}), make(chan struct{})
	finish := sync.OnceFunc(func() { close(release) })
	defer finish()
	server := newHTTPServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = w.Write([]byte("completed"))
	}))
	defer server.Close()
	server.RegisterOnShutdown(func() { close(shuttingDown) })
	done := make(chan error, 1)
	go func() { done <- serve(ctx, server, listener) }()
	requestDone := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			defer resp.Body.Close()
			var body []byte
			body, err = io.ReadAll(resp.Body)
			if err == nil && string(body) != "completed" {
				err = io.ErrUnexpectedEOF
			}
		}
		requestDone <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not reach the handler")
	}
	cancel()
	select {
	case <-shuttingDown:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not start")
	}
	select {
	case err := <-done:
		t.Fatalf("serve returned before active request finished: %v", err)
	default:
	}
	finish()
	for _, result := range []<-chan error{requestDone, done} {
		select {
		case err := <-result:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("request or shutdown did not finish")
		}
	}
}

func TestServeShutsDownGracefullyOnContextCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := newHTTPServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	done := make(chan error, 1)
	go func() { done <- serve(ctx, server, listener) }()

	client := &http.Client{Timeout: time.Second}
	var response *http.Response
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		response, err = client.Get("http://" + listener.Addr().String())
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("server did not start: %v", err)
	}
	_ = response.Body.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve returned shutdown error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not shut down")
	}
}
