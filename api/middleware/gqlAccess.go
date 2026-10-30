package middleware

import (
	"context"

	"at.ourproject/vfeeg-backend/logging"
	"github.com/99designs/gqlgen/graphql"
)

// GQLAccessOperation is a gqlgen operation middleware (handler.Server.AroundOperations):
// it puts the GraphQL operation name into the access line, since every
// GraphQL call is the one route /query.
func GQLAccessOperation(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
	if graphql.HasOperationContext(ctx) {
		if op := graphql.GetOperationContext(ctx).Operation; op != nil {
			logging.SetOperation(ctx, op.Name)
		}
	}
	return next(ctx)
}
