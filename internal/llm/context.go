package llm

import "context"

type routeDecisionContextKey struct{}

func WithRouteDecision(ctx context.Context, decision RouteDecision) context.Context {
	return context.WithValue(ctx, routeDecisionContextKey{}, decision)
}

func RouteDecisionFromContext(ctx context.Context) (RouteDecision, bool) {
	decision, ok := ctx.Value(routeDecisionContextKey{}).(RouteDecision)
	return decision, ok
}
