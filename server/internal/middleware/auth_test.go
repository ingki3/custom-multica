package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/multica-ai/multica/server/internal/auth"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/redis/go-redis/v9"
)

// newRedisTestClient connects to REDIS_TEST_URL, flushes, and skips when
// unset — same gating pattern the rest of the suite uses for Redis-backed
// tests, so `go test ./...` works on a stock laptop without a Redis.
func newRedisTestClient(t *testing.T) *redis.Client {
	t.Helper()
	url := os.Getenv("REDIS_TEST_URL")
	if url == "" {
		t.Skip("REDIS_TEST_URL not set")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatalf("parse REDIS_TEST_URL: %v", err)
	}
	rdb := redis.NewClient(opts)
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("REDIS_TEST_URL unreachable: %v", err)
	}
	if err := rdb.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flushdb: %v", err)
	}
	t.Cleanup(func() {
		rdb.FlushDB(context.Background())
		rdb.Close()
	})
	return rdb
}

func generateToken(claims jwt.MapClaims, secret []byte) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, _ := token.SignedString(secret)
	return s
}

func validClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"sub":   "test-user-id",
		"email": "test@multica.ai",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}
}

// authMiddleware returns the Auth middleware with nil queries (JWT-only tests).
func authMiddleware(next http.Handler) http.Handler {
	return Auth(nil, nil)(next)
}

func TestAuth_MissingHeader(t *testing.T) {
	handler := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/api/me", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if body := w.Body.String(); body != `{"error":"missing authorization"}`+"\n" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestAuth_NoBearerPrefix(t *testing.T) {
	handler := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Token some-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	// Non-Bearer Authorization header with no cookie falls through to "missing authorization".
	if body := w.Body.String(); body != `{"error":"missing authorization"}`+"\n" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestAuth_InvalidToken(t *testing.T) {
	handler := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer not-a-valid-jwt")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_ExpiredToken(t *testing.T) {
	handler := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	claims := validClaims()
	claims["exp"] = time.Now().Add(-time.Hour).Unix()
	token := generateToken(claims, auth.JWTSecret())

	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_WrongSecret(t *testing.T) {
	handler := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	token := generateToken(validClaims(), []byte("wrong-secret"))

	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_WrongSigningMethod(t *testing.T) {
	handler := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	// Use "none" signing method
	token := jwt.NewWithClaims(jwt.SigningMethodNone, validClaims())
	s, _ := token.SignedString(jwt.UnsafeAllowNoneSignatureType)

	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+s)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_ValidToken(t *testing.T) {
	var gotUserID, gotEmail string
	handler := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = r.Header.Get("X-User-ID")
		gotEmail = r.Header.Get("X-User-Email")
		w.WriteHeader(http.StatusOK)
	}))

	token := generateToken(validClaims(), auth.JWTSecret())

	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotUserID != "test-user-id" {
		t.Fatalf("expected X-User-ID 'test-user-id', got '%s'", gotUserID)
	}
	if gotEmail != "test@multica.ai" {
		t.Fatalf("expected X-User-Email 'test@multica.ai', got '%s'", gotEmail)
	}
}

func TestAuth_MissingClaims(t *testing.T) {
	handler := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	// Token with no sub or email claims, only exp
	claims := jwt.MapClaims{
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	token := generateToken(claims, auth.JWTSecret())

	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_InvalidPAT(t *testing.T) {
	handler := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer mul_invalid_token_here")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestAuth_PATCacheHit pins the optimization: when the PAT cache already
// holds an entry for this token, the middleware MUST NOT call into queries
// — it short-circuits before the DB lookup and the last_used_at update.
//
// We exploit that by passing nil queries: a cache miss would dereference
// the nil and panic; a cache hit must not. Reaching the next handler with
// the cached user_id therefore proves the short-circuit fired.
func TestAuth_PATCacheHit(t *testing.T) {
	rdb := newRedisTestClient(t)
	cache := auth.NewPATCache(rdb)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}

	const rawToken = "mul_cache_hit_test_token"
	hash := auth.HashToken(rawToken)
	cache.Set(context.Background(), hash, "cached-user-id", auth.AuthCacheTTL)

	var gotUserID string
	mw := Auth(nil, cache) // nil queries — only safe on cache hit
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = r.Header.Get("X-User-ID")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on cache hit, got %d", w.Code)
	}
	if gotUserID != "cached-user-id" {
		t.Fatalf("expected cached X-User-ID, got %q", gotUserID)
	}
}

func TestAuth_TaskToken(t *testing.T) {
	pool := openPool(t)
	defer pool.Close()
	queries := db.New(pool)
	ctx := context.Background()

	stamp := time.Now().UnixNano()
	email := "task-token-auth-test@example.com"
	slug := "task-token-auth-test"
	_, _ = pool.Exec(ctx, `DELETE FROM workspace WHERE slug = $1`, slug)
	_, _ = pool.Exec(ctx, `DELETE FROM "user" WHERE email = $1`, email)
	defer pool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	defer pool.Exec(context.Background(), `DELETE FROM "user" WHERE email = $1`, email)

	var userID, workspaceID, runtimeID, agentID, taskID string
	if err := pool.QueryRow(ctx, `INSERT INTO "user" (name, email) VALUES ($1, $2) RETURNING id`, "Task Token Auth", email).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO workspace (name, slug, description, issue_prefix) VALUES ($1, $2, '', 'TTA') RETURNING id`, "Task Token Auth", slug).Scan(&workspaceID); err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, workspaceID, userID); err != nil {
		t.Fatalf("insert member: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO agent_runtime (workspace_id, daemon_id, name, runtime_mode, provider, status, device_info, metadata, last_seen_at, owner_id)
		VALUES ($1, NULL, $2, 'local', 'test', 'online', '', '{}'::jsonb, now(), $3)
		RETURNING id
	`, workspaceID, "Task Token Auth Runtime", userID).Scan(&runtimeID); err != nil {
		t.Fatalf("insert runtime: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO agent (workspace_id, name, description, runtime_mode, runtime_config, runtime_id, visibility, max_concurrent_tasks, owner_id)
		VALUES ($1, $2, '', 'local', '{}'::jsonb, $3, 'workspace', 1, $4)
		RETURNING id
	`, workspaceID, "Task Token Auth Agent", runtimeID, userID).Scan(&agentID); err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO agent_task_queue (agent_id, runtime_id, status, priority)
		VALUES ($1, $2, 'running', 1)
		RETURNING id
	`, agentID, runtimeID).Scan(&taskID); err != nil {
		t.Fatalf("insert task: %v", err)
	}

	rawToken := "mat_auth_test_token_" + time.Unix(0, stamp).Format("150405.000000000")
	if _, err := pool.Exec(ctx, `
		INSERT INTO task_token (token_hash, task_id, agent_id, workspace_id, user_id, expires_at)
		VALUES ($1, $2, $3, $4, $5, now() + interval '1 hour')
	`, auth.HashToken(rawToken), taskID, agentID, workspaceID, userID); err != nil {
		t.Fatalf("insert task token: %v", err)
	}

	var gotUserID, gotAgentID, gotTaskID, gotWorkspaceID, gotActorType string
	handler := Auth(queries, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = r.Header.Get("X-User-ID")
		gotAgentID = r.Header.Get("X-Agent-ID")
		gotTaskID = r.Header.Get("X-Task-ID")
		gotWorkspaceID = r.Header.Get("X-Workspace-ID")
		gotActorType = r.Header.Get("X-Actor-Type")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotUserID != userID || gotAgentID != agentID || gotTaskID != taskID || gotWorkspaceID != workspaceID || gotActorType != "agent" {
		t.Fatalf("unexpected headers user=%q agent=%q task=%q workspace=%q actor=%q", gotUserID, gotAgentID, gotTaskID, gotWorkspaceID, gotActorType)
	}
}
