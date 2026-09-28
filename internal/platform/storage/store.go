package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

var (
	ErrTooLarge     = errors.New("archive exceeds the accepted compressed size")
	ErrHashMismatch = errors.New("stored bytes differ from the capture manifest")
)

type Store struct {
	Client *s3.Client
	Bucket string
}
type Receipt struct {
	Key    string `json:"key"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

func New(endpoint, bucket, region, accessKey, secretKey string) *Store {
	return &Store{Bucket: bucket, Client: s3.New(s3.Options{
		Region: region, BaseEndpoint: aws.String(endpoint), UsePathStyle: true,
		Credentials: credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		HTTPClient:  &http.Client{Timeout: 3 * time.Minute}, Retryer: aws.NopRetryer{},
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})}
}

func (s *Store) Capture(ctx context.Context, r io.Reader, prefix string, limit int64) (Receipt, error) {
	if limit <= 0 {
		return Receipt{}, errors.New("capture limit must be positive")
	}
	file, err := os.CreateTemp("", "peergit-archive-*")
	if err != nil {
		return Receipt{}, err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(r, limit+1))
	if err != nil {
		return Receipt{}, err
	}
	if n > limit {
		return Receipt{}, ErrTooLarge
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return Receipt{}, err
	}
	receipt := Receipt{Key: prefix + "/" + rand.Text(), Bytes: n, SHA256: hex.EncodeToString(hash.Sum(nil))}
	_, err = s.Client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.Bucket), Key: aws.String(receipt.Key), Body: file, ContentLength: aws.Int64(n), IfNoneMatch: aws.String("*"), ContentType: aws.String("application/octet-stream")})
	if err != nil {
		return receipt, err
	}
	return receipt, s.Verify(ctx, receipt)
}

func (s *Store) Verify(ctx context.Context, receipt Receipt) error {
	readCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	out, err := s.Client.GetObject(readCtx, &s3.GetObjectInput{Bucket: aws.String(s.Bucket), Key: aws.String(receipt.Key)})
	if err != nil {
		return err
	}
	defer out.Body.Close()
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(out.Body, receipt.Bytes+1))
	if err != nil {
		return err
	}
	if n != receipt.Bytes || hex.EncodeToString(hash.Sum(nil)) != receipt.SHA256 {
		return ErrHashMismatch
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.Client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.Bucket), Key: aws.String(key)})
	return err
}
func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.Client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.Bucket), Key: aws.String(key)})
	if err == nil {
		return true, nil
	}
	if ErrorCode(err) == "NotFound" || ErrorCode(err) == "NoSuchKey" {
		return false, nil
	}
	return false, err
}
func ErrorCode(err error) string {
	var e smithy.APIError
	if errors.As(err, &e) {
		return e.ErrorCode()
	}
	return ""
}
