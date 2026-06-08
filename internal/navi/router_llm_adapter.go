package navi

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/open-navi/navi/internal/llm"
	navitool "github.com/open-navi/navi/internal/tool"
)

type llmCallbacksRouter struct {
	list   func(ctx context.Context) (any, error)
	getAct func(ctx context.Context) (provider, model string, err error)
	setAct func(ctx context.Context, provider, model string) (providerOut, modelOut string, err error)
	route  func(ctx context.Context, req llm.RouteRequest) (llm.RouteDecision, error)
}

func newLLMCallbacksRouter(cfg Config) navitool.RouterLLMService {
	if cfg.ListLLMs == nil && cfg.GetActiveLLM == nil && cfg.SetActiveLLM == nil && cfg.RouteLLM == nil {
		return nil
	}
	return &llmCallbacksRouter{
		list:   cfg.ListLLMs,
		getAct: cfg.GetActiveLLM,
		setAct: cfg.SetActiveLLM,
		route:  cfg.RouteLLM,
	}
}

func (r *llmCallbacksRouter) Catalog(ctx context.Context) (llm.LLMCatalog, error) {
	if r.list == nil {
		return llm.LLMCatalog{}, nil
	}
	v, err := r.list(ctx)
	if err != nil {
		return llm.LLMCatalog{}, err
	}
	if c, ok := v.(llm.LLMCatalog); ok {
		return c, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return llm.LLMCatalog{}, err
	}
	var c llm.LLMCatalog
	if err := json.Unmarshal(b, &c); err != nil {
		return llm.LLMCatalog{}, err
	}
	return c, nil
}

func (r *llmCallbacksRouter) GetActive(ctx context.Context) (llm.Active, error) {
	if r.getAct == nil {
		return llm.Active{}, fmt.Errorf("llm router get_active is not configured")
	}
	p, m, err := r.getAct(ctx)
	return llm.Active{Provider: p, Model: m}, err
}

func (r *llmCallbacksRouter) SetActive(ctx context.Context, provider, model string) (llm.Active, error) {
	if r.setAct == nil {
		return llm.Active{}, fmt.Errorf("llm router set_active is not configured")
	}
	po, mo, err := r.setAct(ctx, provider, model)
	return llm.Active{Provider: po, Model: mo}, err
}

func (r *llmCallbacksRouter) Route(ctx context.Context, req llm.RouteRequest) (llm.RouteDecision, error) {
	if r.route == nil {
		return llm.RouteDecision{}, fmt.Errorf("llm router route is not configured")
	}
	return r.route(ctx, req)
}
