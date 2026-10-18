// Package auth is the dashboard's login: one admin account in Postgres, a
// bcrypt hash, and a signed session token in an httpOnly, SameSite=Strict
// cookie.
package auth

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const CookieName = "pf_session"

var ErrInvalidCredentials = errors.New("invalid email or password")

type Service struct {
	db     *pgxpool.Pool
	secret []byte
	ttl    time.Duration
	secure bool
}

func New(db *pgxpool.Pool, secret string, ttl time.Duration, secureCookies bool) *Service {
	return &Service{db: db, secret: []byte(secret), ttl: ttl, secure: secureCookies}
}

type Admin struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

// Bootstrap creates the admin from ADMIN_EMAIL / ADMIN_PASSWORD when the
// table is empty. It never overwrites an existing account, so a password
// changed in the dashboard survives restarts.
func (s *Service) Bootstrap(ctx context.Context, email, password string) (created bool, err error) {
	var n int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM admin_users`).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	if email == "" || len(password) < 10 {
		return false, errors.New("no admin account yet: set ADMIN_EMAIL and ADMIN_PASSWORD (10+ characters)")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return false, err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO admin_users (email, password_hash) VALUES ($1, $2)`,
		strings.ToLower(strings.TrimSpace(email)), string(hash))
	return err == nil, err
}

func (s *Service) Login(ctx context.Context, email, password string) (Admin, error) {
	var a Admin
	var hash string
	err := s.db.QueryRow(ctx, `SELECT id, email, password_hash FROM admin_users WHERE email = $1`,
		strings.ToLower(strings.TrimSpace(email))).Scan(&a.ID, &a.Email, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		// Spend the same time as a real comparison so timing does not reveal
		// whether the email exists.
		_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(password))
		return a, ErrInvalidCredentials
	}
	if err != nil {
		return a, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return a, ErrInvalidCredentials
	}
	return a, nil
}

func (s *Service) ChangePassword(ctx context.Context, id int64, current, next string) error {
	var hash string
	if err := s.db.QueryRow(ctx, `SELECT password_hash FROM admin_users WHERE id = $1`, id).Scan(&hash); err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(current)) != nil {
		return ErrInvalidCredentials
	}
	if len(next) < 10 {
		return errors.New("new password must be at least 10 characters")
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `UPDATE admin_users SET password_hash = $1, updated_at = now() WHERE id = $2`, string(newHash), id)
	return err
}

var dummyHash = sync.OnceValue(func() []byte {
	h, _ := bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcrypt.DefaultCost)
	return h
})

type claims struct {
	Email string `json:"email"`
	jwt.RegisteredClaims
}

func (s *Service) SetSession(w http.ResponseWriter, a Admin) error {
	now := time.Now()
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		Email: a.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(a.ID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
		},
	}).SignedString(s.secret)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    signed,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(s.ttl.Seconds()),
	})
	return nil
}

func (s *Service) ClearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", HttpOnly: true, Secure: s.secure,
		SameSite: http.SameSiteStrictMode, MaxAge: -1})
}

type ctxKey struct{}

// Require rejects requests without a valid session. Mutating requests must
// also carry X-Requested-With, which a cross-site form cannot set — a second
// line behind SameSite=Strict.
func (s *Service) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err != nil {
			unauthorized(w)
			return
		}
		var cl claims
		_, err = jwt.ParseWithClaims(c.Value, &cl, func(t *jwt.Token) (any, error) { return s.secret, nil },
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
		if err != nil {
			unauthorized(w)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("X-Requested-With") != "portfolio" {
			http.Error(w, `{"error":"missing X-Requested-With header"}`, http.StatusForbidden)
			return
		}
		id, _ := strconv.ParseInt(cl.Subject, 10, 64)
		// A password change revokes every session issued before it.
		var changed time.Time
		if err := s.db.QueryRow(r.Context(), `SELECT updated_at FROM admin_users WHERE id = $1`, id).Scan(&changed); err != nil ||
			cl.IssuedAt == nil || cl.IssuedAt.Time.Before(changed.Truncate(time.Second)) {
			unauthorized(w)
			return
		}
		a := Admin{ID: id, Email: cl.Email}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, a)))
	})
}

// SignedIn reports a valid dashboard session cookie on r, without the
// revocation lookup Require does: good enough to tell the owner's own visits
// apart in analytics, never for access control.
func (s *Service) SignedIn(r *http.Request) bool {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return false
	}
	var cl claims
	_, err = jwt.ParseWithClaims(c.Value, &cl, func(t *jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	return err == nil
}

func FromContext(ctx context.Context) Admin {
	a, _ := ctx.Value(ctxKey{}).(Admin)
	return a
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"Sign in to continue."}`))
}

// Throttle is a small fixed-window limiter keyed by client IP.
type Throttle struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string]*bucket
}

type bucket struct {
	count int
	reset time.Time
}

func NewThrottle(limit int, window time.Duration) *Throttle {
	return &Throttle{limit: limit, window: window, hits: map[string]*bucket{}}
}

// Allow records a hit and reports whether the key is still under the limit.
func (t *Throttle) Allow(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if len(t.hits) > 10_000 {
		for k, b := range t.hits {
			if now.After(b.reset) {
				delete(t.hits, k)
			}
		}
	}
	b, ok := t.hits[key]
	if !ok || now.After(b.reset) {
		b = &bucket{reset: now.Add(t.window)}
		t.hits[key] = b
	}
	b.count++
	return b.count <= t.limit
}
