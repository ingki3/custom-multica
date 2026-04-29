package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/auth"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// bootstrapAdminToken checks the MULTICA_BOOTSTRAP_EMAIL env var. If set and
// the user has no active personal access token, it creates the user (if needed)
// and generates a long-lived PAT printed to stdout. This lets self-hosters run
// `multica setup self-host` without a browser login round-trip.
//
// The token is printed exactly once — on the first server start with that email.
// Subsequent starts detect the existing token and skip silently.
func bootstrapAdminToken(ctx context.Context, queries *db.Queries) {
	email := strings.TrimSpace(os.Getenv("MULTICA_BOOTSTRAP_EMAIL"))
	if email == "" {
		return
	}

	// Find or create the user.
	user, err := queries.GetUserByEmail(ctx, email)
	if err != nil {
		// User doesn't exist — create.
		name := strings.Split(email, "@")[0]
		user, err = queries.CreateUser(ctx, db.CreateUserParams{
			Name:  name,
			Email: email,
		})
		if err != nil {
			slog.Error("bootstrap: failed to create user", "email", email, "error", err)
			return
		}
		slog.Info("bootstrap: created user", "email", email, "user_id", user.ID)
	}

	// Check if the user already has an active (non-revoked, non-expired) PAT.
	tokens, err := queries.ListPersonalAccessTokensByUser(ctx, user.ID)
	if err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("bootstrap: failed to list tokens", "error", err)
		return
	}
	now := time.Now()
	for _, t := range tokens {
		if t.Revoked {
			continue
		}
		if t.ExpiresAt.Valid && t.ExpiresAt.Time.Before(now) {
			continue
		}
		if t.Name == "bootstrap" {
			// Already has an active bootstrap token — skip.
			slog.Info("bootstrap: admin token already exists, skipping", "email", email)
			return
		}
	}

	// Generate a new PAT.
	rawToken, err := auth.GeneratePATToken()
	if err != nil {
		slog.Error("bootstrap: failed to generate token", "error", err)
		return
	}

	expiresAt := pgtype.Timestamptz{
		Time:  now.AddDate(1, 0, 0), // 1 year
		Valid: true,
	}
	prefix := rawToken
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}

	_, err = queries.CreatePersonalAccessToken(ctx, db.CreatePersonalAccessTokenParams{
		UserID:    user.ID,
		Name:      "bootstrap",
		TokenHash: auth.HashToken(rawToken),
		TokenPrefix: prefix,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		slog.Error("bootstrap: failed to create token", "error", err)
		return
	}

	// Print to stdout so the user can copy it.
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(os.Stderr, "  Bootstrap admin token (save this — it is shown only once):\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  MULTICA_TOKEN=%s\n", rawToken)
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  User:    %s (%s)\n", user.Name, user.Email)
	fmt.Fprintf(os.Stderr, "  Expires: %s\n", expiresAt.Time.Format("2006-01-02"))
	fmt.Fprintf(os.Stderr, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  Use it with the CLI:\n")
	fmt.Fprintf(os.Stderr, "    export MULTICA_TOKEN=%s\n", rawToken)
	fmt.Fprintf(os.Stderr, "    multica setup self-host\n")
	fmt.Fprintf(os.Stderr, "\n")

	slog.Info("bootstrap: admin token created", "email", email)
}
