// Package storage keeps uploaded files (images, the CV) in the Garage S3
// store. The bucket stays private: every object is streamed through the app
// at /media/<key>, so nothing about shared infrastructure has to change.
//
// Keys are content-addressed (sha256 prefix), so a URL never changes meaning
// and can be cached as immutable.
package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/devileya/my-portfolio/internal/config"
)

var (
	ErrUnavailable = errors.New("object storage is not configured")
	ErrNotFound    = errors.New("object not found")
)

type Store struct {
	client  *minio.Client
	bucket  string
	timeout time.Duration

	mu    sync.Mutex
	ready bool
}

type Object struct {
	Key         string `json:"key"`
	URL         string `json:"url"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
}

// New returns nil (not an error) when storage is not configured, so the site
// can still run and answer uploads with a clear 503.
func New(cfg config.Config) (*Store, error) {
	if !cfg.StorageConfigured() {
		return nil, nil
	}
	u, err := url.Parse(cfg.S3Endpoint)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("invalid S3_ENDPOINT %q", cfg.S3Endpoint)
	}
	// A bounded transport: the endpoint is a VPN address, and an SDK's
	// default patience turns a dead link into a hung <img> instead of an error.
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ResponseHeaderTimeout: cfg.S3Timeout,
		TLSHandshakeTimeout:   5 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConnsPerHost:   16,
	}
	client, err := minio.New(u.Host, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.S3AccessKey, cfg.S3SecretKey, ""),
		Secure:       u.Scheme == "https",
		Region:       cfg.S3Region, // Garage rejects signatures for any other region
		BucketLookup: minio.BucketLookupPath,
		Transport:    transport,
		MaxRetries:   1,
	})
	if err != nil {
		return nil, err
	}
	return &Store{client: client, bucket: cfg.S3Bucket, timeout: cfg.S3Timeout}, nil
}

// ensureBucket creates the bucket on first use. Created private.
func (s *Store) ensureBucket(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ready {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	ok, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check bucket %s: %w", s.bucket, err)
	}
	if !ok {
		if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("create bucket %s: %w", s.bucket, err)
		}
	}
	// Only success is remembered, so a transient failure is retried next time.
	s.ready = true
	return nil
}

// Put stores data under prefix/<sha256[:20]><ext> and returns its descriptor.
func (s *Store) Put(ctx context.Context, prefix, ext, contentType string, data []byte) (Object, error) {
	if s == nil {
		return Object{}, ErrUnavailable
	}
	if err := s.ensureBucket(ctx); err != nil {
		return Object{}, err
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	key := fmt.Sprintf("%s/%s%s", strings.Trim(prefix, "/"), hash[:20], ext)

	ctx, cancel := context.WithTimeout(ctx, 4*s.timeout)
	defer cancel()
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType:  contentType,
		CacheControl: "public, max-age=31536000, immutable",
	})
	if err != nil {
		return Object{}, fmt.Errorf("upload %s: %w", key, err)
	}
	return Object{Key: key, URL: URL(key), ContentType: contentType, Size: int64(len(data)), SHA256: hash}, nil
}

type Reader struct {
	io.ReadCloser
	ContentType string
	Size        int64
	ETag        string
}

// Get opens an object for streaming. The caller must Close it.
func (s *Store) Get(ctx context.Context, key string) (*Reader, error) {
	if s == nil {
		return nil, ErrUnavailable
	}
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	// GetObject is lazy; Stat forces the request so a missing key is an error
	// here rather than halfway through writing a response.
	info, err := obj.Stat()
	if err != nil {
		obj.Close()
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &Reader{ReadCloser: obj, ContentType: info.ContentType, Size: info.Size, ETag: info.ETag}, nil
}

func (s *Store) Delete(ctx context.Context, key string) error {
	if s == nil {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

// URL is the public path an object is served from.
func URL(key string) string {
	if key == "" {
		return ""
	}
	return "/media/" + key
}
