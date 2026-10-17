package middleware

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/sirupsen/logrus"
)

// F1, step 1 (log only): GQLProtect marks requests whose token has no
// EEG_ADMIN group; GQLAdminCheckLog writes one WARN line per GraphQL
// operation of such a request. Nothing is blocked; enforcing the role
// follows after the log period.

var gqlNonAdminCtxKey = &contextKey{"gql-non-admin"}

// gqlNonAdmin holds the token data that is logged. No e-mail, no username,
// no token.
type gqlNonAdmin struct {
	sub               string
	azp               string
	accessGroupsCount int
	superuser         bool
}

func withGQLNonAdmin(ctx context.Context, claims *PlatformClaims, superuser bool) context.Context {
	return context.WithValue(ctx, gqlNonAdminCtxKey, &gqlNonAdmin{
		sub:               claims.Subject,
		azp:               claims.Authorized,
		accessGroupsCount: len(claims.AccessGroups),
		superuser:         superuser,
	})
}

// GQLAdminCheckLog is a gqlgen operation middleware (handler.Server.AroundOperations).
func GQLAdminCheckLog(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
	if m, ok := ctx.Value(gqlNonAdminCtxKey).(*gqlNonAdmin); ok {
		fields := logrus.Fields{
			"tenant":              ForContextTenant(ctx),
			"azp":                 m.azp,
			"sub":                 m.sub,
			"access_groups_count": m.accessGroupsCount,
			"superuser":           m.superuser,
		}
		if graphql.HasOperationContext(ctx) {
			if op := graphql.GetOperationContext(ctx).Operation; op != nil {
				fields["operation_type"] = string(op.Operation)
				fields["operation_name"] = op.Name
			}
		}
		logrus.WithFields(fields).Warn("gql-admin-check: non-admin token")
	}
	return next(ctx)
}
