package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"pidex/internal/store/dbq"
)

type Principal struct {
	TeamID uuid.UUID
	UserID uuid.UUID
	Role   string
}

func (p Principal) CanWrite() bool { return p.Role != "viewer" }
func (p Principal) CanAdmin() bool { return p.Role == "owner" || p.Role == "admin" }

type principalKey struct{}

func principalFrom(ctx context.Context) Principal {
	p, _ := ctx.Value(principalKey{}).(Principal)
	return p
}

const tokenPrefix = "cbk_"

// NewToken returns a random API token and the hash to store for it.
func NewToken() (token string, hash []byte) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	token = tokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(token))
	return token, sum[:]
}

// AuthInterceptor resolves the bearer token to a team member. Every query
// downstream is scoped by the principal's team.
func AuthInterceptor(q *dbq.Queries) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			token, ok := strings.CutPrefix(req.Header().Get("Authorization"), "Bearer ")
			if !ok || !strings.HasPrefix(token, tokenPrefix) {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing bearer token"))
			}
			sum := sha256.Sum256([]byte(token))
			row, err := q.GetPrincipalByToken(ctx, sum[:])
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid token"))
			}
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			ctx = context.WithValue(ctx, principalKey{}, Principal{TeamID: row.TeamID, UserID: row.UserID, Role: row.Role})
			return next(ctx, req)
		}
	}
}

func requireWrite(p Principal) error {
	if !p.CanWrite() {
		return connect.NewError(connect.CodePermissionDenied, errors.New("read-only member"))
	}
	return nil
}
