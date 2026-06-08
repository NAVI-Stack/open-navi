package llm

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/ceoai/navi/internal/config"
)

// EnrichOllamaFromAPI fetches the list of models from Ollama's /api/tags and
// replaces the ollama provider's Models in the catalog with the live list.
// If the catalog has no ollama provider, it does nothing and returns nil.
// baseURL should be the Ollama root (e.g. http://localhost:11434); /v1 is stripped if present.
func EnrichOllamaFromAPI(ctx context.Context, catalog *LLMCatalog, baseURL string) error {
	if catalog == nil || strings.TrimSpace(baseURL) == "" {
		return nil
	}

	var ollamaIdx int
	found := false
	for i := range catalog.Providers {
		if catalog.Providers[i].Key == "ollama" {
			ollamaIdx = i
			found = true
			break
		}
	}
	if !found {
		return nil
	}

	llmCfg := configForOllamaCatalog(baseURL)
	p, err := BuildProviderByKey(&llmCfg, "ollama")
	if err != nil {
		return err
	}
	local, ok := p.(LocalLifecycleProvider)
	if !ok {
		return fmt.Errorf("llm: registered ollama provider does not implement local lifecycle")
	}
	descriptors, err := local.ListModels(ctx)
	if err != nil {
		return err
	}

	modelSet := make(map[string]struct{})
	for _, d := range descriptors {
		name := strings.TrimSpace(d.Name)
		if name != "" {
			modelSet[name] = struct{}{}
		}
	}
	names := make([]string, 0, len(modelSet))
	for n := range modelSet {
		names = append(names, n)
	}
	sort.Strings(names)

	models := make([]LLMModelInfo, 0, len(names))
	for _, n := range names {
		models = append(models, LLMModelInfo{Name: n})
	}
	catalog.Providers[ollamaIdx].Models = models
	return nil
}

func configForOllamaCatalog(baseURL string) config.LLMConfig {
	return config.LLMConfig{OllamaURL: baseURL}
}

// VerifyOllamaModel checks if a model exists in the public Ollama registry.
// model should be in the format "name", "name:tag", or "namespace/name:tag".
func VerifyOllamaModel(ctx context.Context, model string) (bool, error) {
	if model == "" {
		return false, nil
	}

	namespace := "library"
	name := model
	tag := "latest"

	if strings.Contains(name, ":") {
		parts := strings.SplitN(name, ":", 2)
		name = parts[0]
		tag = parts[1]
	}
	if strings.Contains(name, "/") {
		parts := strings.SplitN(name, "/", 2)
		namespace = parts[0]
		name = parts[1]
	}

	url := "https://registry.ollama.ai/v2/" + namespace + "/" + name + "/manifests/" + tag
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false, err
	}
	// The registry requires a Docker/OCI accept header to return a proper manifest summary
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("registry returned status: %s", resp.Status)
}
