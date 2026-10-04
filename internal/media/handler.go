package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"peergit/internal/identity"
	"peergit/internal/platform/errormanager"
	"peergit/internal/platform/http/request"
	"peergit/internal/platform/http/response"
	"peergit/internal/platform/storage"
)

const (
	maxUploadBytes = 5 << 20
	maxPixels      = 20_000_000
)

type Handler struct {
	pool        *pgxpool.Pool
	store       *storage.Store
	auth        *identity.Handler
	err         *errormanager.Manager
	uploadSlots chan struct{}
}

func NewHandler(pool *pgxpool.Pool, store *storage.Store, auth *identity.Handler, errors *errormanager.Manager) *Handler {
	return &Handler{pool: pool, store: store, auth: auth, err: errors, uploadSlots: make(chan struct{}, 4)}
}

func (h *Handler) Register(r chi.Router) {
	r.Group(func(private chi.Router) {
		private.Use(h.auth.Middleware)
		private.With(h.auth.RequireCSRF).Post("/media/intents", h.createUploadIntent)
		private.With(h.auth.RequireCSRF).Post("/media/intents/{intent}/content", h.upload)
		private.Get("/media/{id}", h.download)
	})
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	s, ok := identity.CurrentSession(r.Context())
	if !ok {
		h.fail(w, r, http.StatusUnauthorized, "authentication_required", "sign in is required", nil)
		return
	}
	var collegeID, visibility string
	err := h.pool.QueryRow(r.Context(), `SELECT COALESCE(college_id::text,''),visibility FROM media_upload_intents WHERE id=$1 AND user_id=$2 AND consumed_at IS NULL AND expires_at>now()`, chi.URLParam(r, "intent"), s.UserID).Scan(&collegeID, &visibility)
	if err != nil {
		h.fail(w, r, http.StatusForbidden, "upload_intent_invalid", "upload intent is missing, expired, or already used", err)
		return
	}
	tag, err := h.pool.Exec(r.Context(), `UPDATE media_upload_intents SET consumed_at=now() WHERE id=$1 AND user_id=$2 AND consumed_at IS NULL AND expires_at>now()`, chi.URLParam(r, "intent"), s.UserID)
	if err != nil || tag.RowsAffected() != 1 {
		h.fail(w, r, http.StatusConflict, "upload_intent_invalid", "upload intent was already used or expired", err)
		return
	}
	if collegeID != "" && (!s.TermsAccepted || !s.PrivacyAccepted) {
		h.fail(w, r, http.StatusForbidden, "policy_consent_required", "accept the Terms and Privacy Notice to use campus media", nil)
		return
	}
	if h.store == nil {
		h.fail(w, r, http.StatusServiceUnavailable, "media_unavailable", "media storage is not configured", nil)
		return
	}
	// ponytail: one process-wide upload cap protects decode/storage resources; split per-user quotas when measured contention requires it.
	select {
	case h.uploadSlots <- struct{}{}:
		defer func() { <-h.uploadSlots }()
	default:
		w.Header().Set("Retry-After", "5")
		h.fail(w, r, http.StatusTooManyRequests, "upload_capacity", "too many images are being processed; try again shortly", nil)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+(256<<10))
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			h.fail(w, r, http.StatusRequestEntityTooLarge, "media_too_large", "image exceeds the 5 MiB upload limit", err)
		} else {
			h.fail(w, r, http.StatusBadRequest, "media_invalid", "upload must contain one PNG or JPEG file", err)
		}
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	files := r.MultipartForm.File["file"]
	if len(files) != 1 {
		h.fail(w, r, http.StatusBadRequest, "media_invalid", "upload must contain exactly one image file", nil)
		return
	}
	file, err := files[0].Open()
	if err != nil {
		h.fail(w, r, http.StatusBadRequest, "media_invalid", "uploaded image could not be read", err)
		return
	}
	input, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
	_ = file.Close()
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if len(input) == 0 || len(input) > maxUploadBytes {
		h.fail(w, r, http.StatusRequestEntityTooLarge, "media_too_large", "image exceeds the 5 MiB upload limit", nil)
		return
	}
	clean, mimeType, err := sanitize(input)
	if err != nil {
		h.fail(w, r, http.StatusUnsupportedMediaType, "media_type_unsupported", "only valid PNG and JPEG images are accepted", err)
		return
	}
	if visibility == "campus" {
		var consent bool
		if err := h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM consent_records WHERE user_id=$1 AND purpose='profile_discovery' AND revoked_at IS NULL)`, s.UserID).Scan(&consent); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if !consent {
			h.fail(w, r, http.StatusForbidden, "consent_required", "campus-visible media requires profile discovery consent", nil)
			return
		}
	}
	prefix := "profile/" + s.UserID
	if collegeID != "" {
		prefix = "quarantine/" + collegeID
	}
	object, err := h.store.Capture(r.Context(), bytes.NewReader(clean), prefix, maxUploadBytes)
	if err != nil {
		h.fail(w, r, http.StatusBadGateway, "media_store_failed", "image could not be stored safely", err)
		return
	}
	hash, err := hex.DecodeString(object.SHA256)
	if err != nil {
		_ = h.store.Delete(context.WithoutCancel(r.Context()), object.Key)
		h.err.Handle(w, r, err)
		return
	}
	var id string
	err = h.pool.QueryRow(r.Context(), `INSERT INTO media(college_id,owner_user_id,object_key,mime_type,byte_size,sha256,scan_status,visibility)
		VALUES(NULLIF($1,'')::uuid,$2,$3,$4,$5,$6,'clean',$7) RETURNING id::text`, collegeID, s.UserID, object.Key, mimeType, object.Bytes, hash, visibility).Scan(&id)
	if err != nil {
		_ = h.store.Delete(context.WithoutCancel(r.Context()), object.Key)
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"id": id, "mime_type": mimeType, "bytes": object.Bytes, "sha256": object.SHA256, "visibility": visibility, "url": "/api/v1/media/" + id})
}

func (h *Handler) createUploadIntent(w http.ResponseWriter, r *http.Request) {
	s, ok := identity.CurrentSession(r.Context())
	if !ok {
		h.fail(w, r, http.StatusUnauthorized, "authentication_required", "sign in is required", nil)
		return
	}
	input, err := request.Decode[struct {
		Visibility string `json:"visibility"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	visibility := input.Visibility
	if visibility == "" {
		visibility = "private"
	}
	if visibility != "private" && visibility != "campus" {
		h.fail(w, r, http.StatusBadRequest, "media_visibility_invalid", "visibility must be private or campus", nil)
		return
	}
	if visibility == "campus" && (s.CollegeID == "" || !s.TermsAccepted || !s.PrivacyAccepted) {
		h.fail(w, r, http.StatusForbidden, "campus_scope_required", "campus-visible media requires verified campus access and policy consent", nil)
		return
	}
	var id string
	err = h.pool.QueryRow(r.Context(), `INSERT INTO media_upload_intents(user_id,college_id,purpose,visibility,expires_at) VALUES($1,NULLIF($2,'')::uuid,'profile_image',$3,now()+interval '5 minutes') RETURNING id::text`, s.UserID, s.CollegeID, visibility).Scan(&id)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"id": id, "expires_in_seconds": 300, "visibility": visibility, "upload_url": "/api/v1/media/intents/" + id + "/content"})
}

func sanitize(input []byte) ([]byte, string, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(input))
	if err != nil || (format != "png" && format != "jpeg") || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return nil, "", errors.New("invalid or unsupported image")
	}
	img, decodedFormat, err := image.Decode(bytes.NewReader(input))
	if err != nil || decodedFormat != format {
		return nil, "", errors.New("image decode failed")
	}
	var output bytes.Buffer
	mimeType := "image/png"
	if format == "png" {
		err = png.Encode(&output, img)
	} else {
		mimeType = "image/jpeg"
		err = jpeg.Encode(&output, img, &jpeg.Options{Quality: 85})
	}
	if err != nil {
		return nil, "", err
	}
	if output.Len() == 0 || output.Len() > maxUploadBytes {
		return nil, "", errors.New("re-encoded image exceeds the size limit")
	}
	return output.Bytes(), mimeType, nil
}

func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	s, ok := identity.CurrentSession(r.Context())
	if !ok {
		h.fail(w, r, http.StatusUnauthorized, "authentication_required", "sign in is required", nil)
		return
	}
	if !s.TermsAccepted || !s.PrivacyAccepted {
		h.fail(w, r, http.StatusForbidden, "policy_consent_required", "accept the Terms and Privacy Notice to view media", nil)
		return
	}
	if h.store == nil {
		h.fail(w, r, http.StatusServiceUnavailable, "media_unavailable", "media storage is not configured", nil)
		return
	}
	var owner, college, key, mimeType string
	var size int64
	err := h.pool.QueryRow(r.Context(), `SELECT owner_user_id::text,COALESCE(college_id::text,''),object_key,mime_type,byte_size FROM media WHERE id=$1 AND scan_status='clean'`, chi.URLParam(r, "id")).Scan(&owner, &college, &key, &mimeType, &size)
	if err != nil {
		h.fail(w, r, http.StatusNotFound, "media_not_found", "media not found", err)
		return
	}
	if owner != s.UserID {
		if s.CollegeID == "" || s.CollegeID != college {
			h.fail(w, r, http.StatusNotFound, "media_not_found", "media not found", nil)
			return
		}
		var consent bool
		if err = h.pool.QueryRow(r.Context(), `SELECT visibility='campus' AND EXISTS(SELECT 1 FROM consent_records WHERE user_id=$1 AND purpose='profile_discovery' AND revoked_at IS NULL) FROM media WHERE id=$2`, owner, chi.URLParam(r, "id")).Scan(&consent); err != nil || !consent {
			h.fail(w, r, http.StatusNotFound, "media_not_found", "media not found", err)
			return
		}
	}
	out, err := h.store.Client.GetObject(r.Context(), &s3.GetObjectInput{Bucket: aws.String(h.store.Bucket), Key: aws.String(key)})
	if err != nil {
		h.fail(w, r, http.StatusBadGateway, "media_unavailable", "media could not be retrieved", err)
		return
	}
	defer out.Body.Close()
	data, err := io.ReadAll(io.LimitReader(out.Body, size+1))
	if err != nil || int64(len(data)) != size {
		h.fail(w, r, http.StatusBadGateway, "media_unavailable", "media could not be verified", err)
		return
	}
	hash := sha256.Sum256(data)
	var expected []byte
	if err = h.pool.QueryRow(r.Context(), `SELECT sha256 FROM media WHERE id=$1`, chi.URLParam(r, "id")).Scan(&expected); err != nil || !bytes.Equal(expected, hash[:]) {
		h.fail(w, r, http.StatusBadGateway, "media_integrity_failed", "media could not be verified", err)
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, status int, code, message string, cause error) {
	h.err.Handle(w, r, errormanager.New(status, code, message, cause))
}
