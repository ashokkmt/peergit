package campus

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type resendWebhookHandler struct {
	pool            *pgxpool.Pool
	secret, hashKey string
	logger          *slog.Logger
}

func NewResendWebhookHandler(pool *pgxpool.Pool, secret, hashKey string, logger *slog.Logger) *resendWebhookHandler {
	return &resendWebhookHandler{pool: pool, secret: secret, hashKey: hashKey, logger: logger}
}

func (h *resendWebhookHandler) Register(r chi.Router) {
	r.Post("/webhooks/resend", h.receive)
}

func (h *resendWebhookHandler) receive(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil || h.secret == "" || len(h.hashKey) < 32 {
		http.Error(w, "webhook unavailable", http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "invalid webhook body", http.StatusRequestEntityTooLarge)
		return
	}
	id, timestamp, signature := r.Header.Get("svix-id"), r.Header.Get("svix-timestamp"), r.Header.Get("svix-signature")
	if !verifyResendSignature(h.secret, id, timestamp, signature, body, time.Now()) {
		http.Error(w, "invalid webhook signature", http.StatusUnauthorized)
		return
	}
	var event struct {
		Type      string    `json:"type"`
		CreatedAt time.Time `json:"created_at"`
		Data      struct {
			EmailID string   `json:"email_id"`
			To      []string `json:"to"`
		} `json:"data"`
	}
	if err = json.Unmarshal(body, &event); err != nil || event.Type == "" || len(event.Data.EmailID) > 200 {
		http.Error(w, "invalid webhook event", http.StatusBadRequest)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	defer tx.Rollback(r.Context())
	tag, err := tx.Exec(r.Context(), `INSERT INTO email_webhook_receipts(event_id,provider_message_id,event_type,occurred_at) VALUES($1,$2,$3,$4) ON CONFLICT(event_id) DO NOTHING`, id, event.Data.EmailID, event.Type, event.CreatedAt)
	if err != nil {
		http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	if tag.RowsAffected() == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}
	status := map[string]string{"email.delivered": "delivered", "email.bounced": "bounced", "email.failed": "failed", "email.suppressed": "suppressed"}[event.Type]
	processed := status == ""
	if status != "" {
		var known bool
		if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM campus_email_deliveries WHERE provider='resend' AND provider_message_id=$1)`, event.Data.EmailID).Scan(&known); err != nil {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		if !known {
			http.Error(w, "email not yet recorded", http.StatusServiceUnavailable)
			return
		}
		tag, err = tx.Exec(r.Context(), `UPDATE campus_email_deliveries SET delivery_status=$2,status_at=$3,last_error_code=NULL WHERE provider='resend' AND provider_message_id=$1 AND (delivery_status IS NULL OR delivery_status='accepted' OR status_at<$3)`, event.Data.EmailID, status, event.CreatedAt)
		if err != nil {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		processed = tag.RowsAffected() > 0
	}
	if event.Type == "email.bounced" || event.Type == "email.complained" || event.Type == "email.suppressed" {
		for _, address := range event.Data.To {
			key := recipientKey(h.hashKey, address)
			if _, err = tx.Exec(r.Context(), `INSERT INTO email_suppressions(recipient_key,reason) VALUES($1,$2) ON CONFLICT(recipient_key) DO UPDATE SET reason=EXCLUDED.reason,recorded_at=clock_timestamp()`, key, suppressionReason(event.Type)); err != nil {
				http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
				return
			}
		}
	}
	if processed {
		if _, err = tx.Exec(r.Context(), `UPDATE email_webhook_receipts SET processed_at=clock_timestamp() WHERE event_id=$1`, id); err != nil {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func verifyResendSignature(secret, id, timestamp, signatures string, body []byte, now time.Time) bool {
	if id == "" || len(id) > 200 || len(body) > 1<<20 {
		return false
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || now.Sub(time.Unix(seconds, 0)) > 5*time.Minute || time.Unix(seconds, 0).Sub(now) > 5*time.Minute {
		return false
	}
	secret = strings.TrimPrefix(secret, "whsec_")
	key, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(id + "." + timestamp + "."))
	_, _ = mac.Write(body)
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	for _, part := range strings.Split(signatures, " ") {
		version, value, ok := strings.Cut(part, ",")
		if ok && version == "v1" && hmac.Equal([]byte(value), []byte(want)) {
			return true
		}
	}
	return false
}

func recipientKey(key, address string) []byte {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(strings.ToLower(strings.TrimSpace(address))))
	return mac.Sum(nil)
}

func suppressionReason(event string) string {
	switch event {
	case "email.bounced":
		return "hard_bounce"
	case "email.complained":
		return "complaint"
	default:
		return "provider_suppressed"
	}
}
