package middleware

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/sirupsen/logrus"
)

// F1, step 1 (log only): GQLProtect marks requests whose token has no
// EEG_ADMIN group; GQLAdminCheckLog writes one INFO line per GraphQL
// operation of such a request. Nothing is blocked; enforcing the role
// follows after the log period.

var gqlNonAdminCtxKey = &contextKey{"gql-non-admin"}

// gqlNonAdmin holds the token data that is logged. No e-mail, no username,
// no token.
type gqlNonAdmin struct {
	sub               string
	azp               string
	accessGroupsCount int
	isUser            bool
	superuser         bool
}

func withGQLNonAdmin(ctx context.Context, claims *PlatformClaims, superuser bool) context.Context {
	return context.WithValue(ctx, gqlNonAdminCtxKey, &gqlNonAdmin{
		sub:               claims.Subject,
		azp:               claims.Authorized,
		accessGroupsCount: len(claims.AccessGroups),
		isUser:            claims.AccessGroups.IsUser(),
		superuser:         superuser,
	})
}

// GQLAdminCheckLog is a gqlgen operation middleware (handler.Server.AroundOperations).
//
// TODO(F1 step 2): temporary log; remove this hook and the context marker when
// /query enforces the admin role.
func GQLAdminCheckLog(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
	if m, ok := ctx.Value(gqlNonAdminCtxKey).(*gqlNonAdmin); ok {
		fields := logrus.Fields{
			"tenant":              ForContextTenant(ctx),
			"azp":                 m.azp,
			"sub":                 m.sub,
			"access_groups_count": m.accessGroupsCount,
			"is_user":             m.isUser,
			"superuser":           m.superuser,
		}
		if graphql.HasOperationContext(ctx) {
			if op := graphql.GetOperationContext(ctx).Operation; op != nil {
				fields["operation_type"] = string(op.Operation)
				fields["operation_name"] = op.Name
			}
		}
		logrus.WithFields(fields).Info("gql-admin-check: non-admin token")
	}
	return next(ctx)
}
