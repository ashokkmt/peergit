package campus

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/resend/resend-go/v4"
	"peergit/internal/platform/jobs"
)

type MailConfig struct {
	Host, From, Username, Password, TLSMode string
	Resend                                  *ResendSender
	SuppressionKey                          string
}

const resendRequestTimeout = 8 * time.Second

// ResendSender owns one SDK and HTTP client for the worker process.
type ResendSender struct {
	client     *resend.Client
	httpClient *http.Client
}

func NewResendSender(apiKey string) *ResendSender {
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	var transport http.RoundTripper = http.DefaultTransport
	if defaultTransport, ok := http.DefaultTransport.(*http.Transport); ok {
		cloned := defaultTransport.Clone()
		cloned.MaxIdleConns = 10
		cloned.MaxIdleConnsPerHost = 2
		cloned.IdleConnTimeout = 90 * time.Second
		transport = cloned
	}
	httpClient := &http.Client{Transport: transport, Timeout: resendRequestTimeout}
	return &ResendSender{client: resend.NewCustomClient(httpClient, apiKey), httpClient: httpClient}
}

func (s *ResendSender) CloseIdleConnections() {
	if s != nil && s.httpClient != nil {
		s.httpClient.CloseIdleConnections()
	}
}

func (s *ResendSender) send(ctx context.Context, deliveryID string, data deliveryPayload) (string, error) {
	return sendResendWithClient(ctx, s.client, deliveryID, data)
}

// SendNotification sends non-auth activity mail with a stable provider key. The caller
// owns suppression checks and persists the returned provider ID before completing its job.
func (s *ResendSender) SendNotification(ctx context.Context, deliveryID, from, to, subject, text, html string) (string, error) {
	if s == nil || s.client == nil || deliveryID == "" || from == "" || !validMail(mailbox(from)) || !validMail(to) {
		return "", errors.New("notification email sender or recipient is unavailable")
	}
	request, err := s.client.Emails.SendWithOptions(ctx, &resend.SendEmailRequest{
		From: from, To: []string{to}, Subject: subject,
		ReplyTo: "support@peergit.com", Text: text, Html: html,
	}, &resend.SendEmailOptions{IdempotencyKey: "peer-notification/" + deliveryID})
	if err != nil {
		var rateLimit *resend.RateLimitError
		if errors.As(err, &rateLimit) {
			seconds, parseErr := strconv.Atoi(rateLimit.RetryAfter)
			if parseErr == nil && seconds > 0 {
				return "", retryAfterError{err: err, delay: time.Duration(seconds) * time.Second}
			}
			return "", err
		}
		var apiError *resend.ResendError
		if errors.As(err, &apiError) && apiError.StatusCode >= 400 && apiError.StatusCode < 500 && apiError.StatusCode != http.StatusConflict {
			return "", &jobs.PermanentError{Code: "resend_rejected", Err: err}
		}
		return "", err
	}
	return request.Id, nil
}

// SendSMTPNotification reuses the local Mailpit/SMTP transport for activity mail.
func SendSMTPNotification(ctx context.Context, cfg MailConfig, to, subject, text, html string) error {
	if cfg.Host == "" || cfg.From == "" || !validMail(to) {
		return errors.New("SMTP host, sender, and valid recipient are required")
	}
	hostname, _, err := net.SplitHostPort(cfg.Host)
	if err != nil {
		return errors.New("SMTP_HOST must be host:port")
	}
	dialer := net.Dialer{Timeout: 8 * time.Second}
	var conn net.Conn
	if cfg.TLSMode == "tls" {
		conn, err = tls.DialWithDialer(&dialer, "tcp", cfg.Host, &tls.Config{ServerName: hostname, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", cfg.Host)
	}
	if err != nil {
		return err
	}
	closed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-closed:
		}
	}()
	defer close(closed)
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	client, err := smtp.NewClient(conn, hostname)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer client.Close()
	if cfg.TLSMode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("SMTP server does not offer STARTTLS")
		}
		if err = client.StartTLS(&tls.Config{ServerName: hostname, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if cfg.Username != "" {
		if err = client.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, hostname)); err != nil {
			return err
		}
	}
	if err = client.Mail(mailbox(cfg.From)); err != nil {
		return err
	}
	if err = client.Rcpt(to); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=\"peergit-notice\"\r\n\r\n--peergit-notice\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n--peergit-notice\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s\r\n--peergit-notice--\r\n", cfg.From, to, strings.ReplaceAll(strings.ReplaceAll(subject, "\r", " "), "\n", " "), text, html)
	if _, err = io.WriteString(writer, body); err != nil {
		_ = writer.Close()
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func validMail(address string) bool {
	parsed, err := mail.ParseAddress(address)
	return err == nil && parsed.Address == strings.TrimSpace(address)
}

func DeliveryHandler(pool *pgxpool.Pool, encryptionKey string, mail MailConfig) jobs.Handler {
	return func(ctx context.Context, claim jobs.Claim) (string, error) {
		var deliveryID string
		if err := json.Unmarshal(claim.Payload, &struct {
			DeliveryID *string `json:"delivery_id"`
		}{DeliveryID: &deliveryID}); err != nil || deliveryID == "" {
			return "", errors.New("verification delivery job is invalid")
		}
		var current bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE id=$1 AND worker=$2 AND generation=$3 AND state='running' AND lease_until>clock_timestamp())`, claim.ID, claim.Worker, claim.Generation).Scan(&current); err != nil {
			return "", err
		}
		if !current {
			return "", jobs.ErrStale
		}
		var challengeID string
		var encrypted []byte
		var to string
		var alreadySent bool
		err := pool.QueryRow(ctx, `SELECT d.challenge_id::text,d.encrypted_payload,ce.email_normalized,d.sent_at IS NOT NULL FROM campus_email_deliveries d JOIN campus_email_challenges ch ON ch.id=d.challenge_id JOIN campus_emails ce ON ce.id=ch.campus_email_id JOIN colleges c ON c.id=ch.college_id JOIN college_domains domain ON domain.college_id=c.id AND domain.domain=split_part(ce.email_normalized,'@',2) AND domain.verified_at IS NOT NULL JOIN users u ON u.id=ch.user_id WHERE d.id=$1 AND d.expires_at>now() AND ch.expires_at>now() AND ch.superseded_at IS NULL AND ch.consumed_at IS NULL AND c.status='active' AND u.status='active' AND u.account_type='unverified' AND u.college_id IS NULL`, deliveryID).Scan(&challengeID, &encrypted, &to, &alreadySent)
		if errors.Is(err, pgx.ErrNoRows) {
			_, _ = pool.Exec(ctx, `UPDATE campus_email_deliveries SET encrypted_payload=''::bytea WHERE id=$1 AND sent_at IS NULL`, deliveryID)
			return "", nil
		}
		if err != nil {
			return "", err
		}
		if alreadySent {
			return "", nil
		}
		decryptor := &Handler{encryptionKey: encryptionKey}
		payload, err := decryptor.decrypt(encrypted)
		if err != nil {
			var permanent *jobs.PermanentError
			if errors.As(err, &permanent) {
				_, updateErr := pool.Exec(ctx, `UPDATE campus_email_deliveries SET encrypted_payload=''::bytea,delivery_status='failed',last_error_code=$2,status_at=clock_timestamp() WHERE id=$1 AND sent_at IS NULL AND EXISTS(SELECT 1 FROM jobs WHERE id=$3 AND worker=$4 AND generation=$5 AND state='running' AND lease_until>clock_timestamp())`, deliveryID, permanent.Code, claim.ID, claim.Worker, claim.Generation)
				if updateErr != nil {
					return "", updateErr
				}
			}
			return "", err
		}
		var data deliveryPayload
		if err = json.Unmarshal(payload, &data); err != nil {
			return "", err
		}
		if data.To != to {
			return "", errors.New("verification recipient no longer matches")
		}
		if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE id=$1 AND worker=$2 AND generation=$3 AND state='running' AND lease_until>clock_timestamp()),EXISTS(SELECT 1 FROM campus_email_challenges ch JOIN campus_email_deliveries d ON d.challenge_id=ch.id JOIN campus_emails ce ON ce.id=ch.campus_email_id JOIN colleges c ON c.id=ch.college_id JOIN college_domains domain ON domain.college_id=c.id AND domain.domain=split_part(ce.email_normalized,'@',2) AND domain.verified_at IS NOT NULL JOIN users u ON u.id=ch.user_id WHERE d.id=$4 AND d.expires_at>now() AND ch.expires_at>now() AND ch.superseded_at IS NULL AND ch.consumed_at IS NULL AND c.status='active' AND u.status='active' AND u.account_type='unverified' AND u.college_id IS NULL)`, claim.ID, claim.Worker, claim.Generation, deliveryID).Scan(&current, &alreadySent); err != nil {
			return "", err
		}
		if !current {
			return "", jobs.ErrStale
		}
		if !alreadySent {
			_, _ = pool.Exec(ctx, `UPDATE campus_email_deliveries SET encrypted_payload=''::bytea WHERE id=$1 AND sent_at IS NULL`, deliveryID)
			return "", nil
		}
		if mail.SuppressionKey != "" {
			var suppressed bool
			if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM email_suppressions WHERE recipient_key=$1)`, recipientKey(mail.SuppressionKey, to)).Scan(&suppressed); err != nil {
				return "", err
			}
			if suppressed {
				_, err := pool.Exec(ctx, `UPDATE campus_email_deliveries SET encrypted_payload=''::bytea,delivery_status='suppressed',status_at=clock_timestamp() WHERE id=$1 AND sent_at IS NULL AND EXISTS(SELECT 1 FROM jobs WHERE id=$2 AND worker=$3 AND generation=$4 AND state='running' AND lease_until>clock_timestamp())`, deliveryID, claim.ID, claim.Worker, claim.Generation)
				return "", err
			}
		}
		provider := "smtp"
		var messageID string
		if mail.Resend != nil {
			provider = "resend"
			messageID, err = mail.Resend.send(ctx, deliveryID, data)
		} else {
			err = sendMail(ctx, mail, data)
		}
		if err != nil {
			return "", err
		}
		tag, err := pool.Exec(ctx, `UPDATE campus_email_deliveries SET sent_at=now(),encrypted_payload=''::bytea,attempts=LEAST(attempts+1,3),provider=$5,provider_message_id=NULLIF($6,''),delivery_status='accepted',status_at=clock_timestamp() WHERE id=$1 AND sent_at IS NULL AND EXISTS(SELECT 1 FROM jobs WHERE id=$2 AND worker=$3 AND generation=$4 AND state='running' AND lease_until>clock_timestamp())`, deliveryID, claim.ID, claim.Worker, claim.Generation, provider, messageID)
		if err == nil && tag.RowsAffected() != 1 {
			return "", jobs.ErrStale
		}
		return "", err
	}
}

func sendResendWithClient(ctx context.Context, client *resend.Client, deliveryID string, data deliveryPayload) (string, error) {
	request, err := client.Emails.SendWithOptions(ctx, &resend.SendEmailRequest{
		From: "PeerGit Accounts <auth@mail.peergit.com>", To: []string{data.To}, Subject: "Verify your PeerGit campus email",
		ReplyTo: "support@peergit.com", Text: fmt.Sprintf("Confirm your campus email using this link (it expires in 10 minutes):\n%s\n\nOr enter this six-digit code: %s", data.Link, data.Code),
		Html: fmt.Sprintf("<p>Confirm your campus email using this link (it expires in 10 minutes):</p><p><a href=\"%s\">Verify campus email</a></p><p>Or enter this six-digit code: <strong>%s</strong></p>", htmlEscape(data.Link), htmlEscape(data.Code)),
	}, &resend.SendEmailOptions{IdempotencyKey: "campus-verification/" + deliveryID})
	if err != nil {
		var rateLimit *resend.RateLimitError
		if errors.As(err, &rateLimit) {
			seconds, parseErr := strconv.Atoi(rateLimit.RetryAfter)
			if parseErr == nil && seconds > 0 {
				return "", retryAfterError{err: err, delay: time.Duration(seconds) * time.Second}
			}
			return "", err
		}
		var apiError *resend.ResendError
		if errors.As(err, &apiError) && apiError.StatusCode >= 400 && apiError.StatusCode < 500 {
			if apiError.StatusCode == http.StatusConflict && apiError.Name == "concurrent_idempotent_requests" {
				return "", err
			}
			code := "resend_rejected"
			if apiError.StatusCode == http.StatusConflict && apiError.Name == "invalid_idempotent_request" {
				code = "resend_idempotency_conflict"
			}
			return "", &jobs.PermanentError{Code: code, Err: err}
		}
		return "", err
	}
	return request.Id, nil
}

type retryAfterError struct {
	err   error
	delay time.Duration
}

func (e retryAfterError) Error() string                              { return e.err.Error() }
func (e retryAfterError) Unwrap() error                              { return e.err }
func (e retryAfterError) RetryAfterDuration(time.Time) time.Duration { return e.delay }

func htmlEscape(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&#39;").Replace(value)
}

func RunDeliveryCleanup(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) {
	clean := func() {
		if _, err := pool.Exec(ctx, `UPDATE campus_email_deliveries SET encrypted_payload=''::bytea WHERE expires_at<=now() AND octet_length(encrypted_payload)>0`); err != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "expired campus email secret cleanup failed", "error", err)
		}
	}
	clean()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			clean()
		}
	}
}

func sendMail(ctx context.Context, cfg MailConfig, data deliveryPayload) error {
	if cfg.Host == "" || cfg.From == "" {
		return errors.New("SMTP host and sender are required")
	}
	hostname, _, err := net.SplitHostPort(cfg.Host)
	if err != nil {
		return errors.New("SMTP_HOST must be host:port")
	}
	dialer := net.Dialer{Timeout: 8 * time.Second}
	var conn net.Conn
	if cfg.TLSMode == "tls" {
		conn, err = tls.DialWithDialer(&dialer, "tcp", cfg.Host, &tls.Config{ServerName: hostname, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", cfg.Host)
	}
	if err != nil {
		return err
	}
	connectionClosed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-connectionClosed:
		}
	}()
	defer close(connectionClosed)
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	client, err := smtp.NewClient(conn, hostname)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer client.Close()
	if cfg.TLSMode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("SMTP server does not offer STARTTLS")
		}
		if err = client.StartTLS(&tls.Config{ServerName: hostname, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if cfg.Username != "" {
		if err = client.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, hostname)); err != nil {
			return err
		}
	}
	if err = client.Mail(mailbox(cfg.From)); err != nil {
		return err
	}
	if err = client.Rcpt(data.To); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: Verify your PeerGit campus email\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nConfirm your campus email using this link (it expires in 10 minutes):\r\n%s\r\n\r\nOr enter this six-digit code: %s\r\n", cfg.From, data.To, data.Link, data.Code)
	if _, err = io.WriteString(writer, body); err != nil {
		_ = writer.Close()
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func mailbox(value string) string {
	if parsed, err := mail.ParseAddress(value); err == nil {
		return parsed.Address
	}
	return strings.TrimSpace(value)
}
