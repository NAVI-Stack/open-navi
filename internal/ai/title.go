package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/open-navi/navi/internal/llm"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// TitleGeneratorInput matches the requested API shape.
type TitleGeneratorInput struct {
	ConversationID          string        `json:"conversation_id,omitempty"`
	Messages                []llm.Message `json:"messages"`
	ExistingTitle           string        `json:"existing_title,omitempty"`
	Mode                    string        `json:"mode"` // create, regenerate, refresh
	TitleManuallyOverridden bool          `json:"title_manually_overridden,omitempty"`
}

// TitleGeneratorResponse matches the requested API shape.
type TitleGeneratorResponse struct {
	Title      string  `json:"title"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

const (
	ModeCreate     = "create"
	ModeRegenerate = "regenerate"
	ModeRefresh    = "refresh"
)

const titleGenerationSystemPrompt = `You generate concise titles for AI chat conversations.

Your job is to infer the user's main intent and produce a short, useful conversation title.

Rules:
- Output only valid JSON.
- Do not include markdown.
- Do not explain yourself outside the JSON.
- The title should usually be 3 to 7 words.
- Prefer specific, concrete titles over vague summaries.
- Capture the user's actual goal, not just the first topic mentioned.
- If the conversation is about designing, building, debugging, planning, reviewing, or deciding something, reflect that action.
- Do not include sensitive personal details, secrets, credentials, private identifiers, addresses, medical details, financial details, or legal details.
- If the user mentions a project, app, repo, or product name, you may include it if it improves clarity.
- Avoid generic titles like "Chat Summary", "AI Discussion", "Project Help", "General Question", "Technology Conversation", "New Chat", or "Conversation".
- Avoid clickbait.
- Avoid emojis.
- Use Title Case for English titles.
- If the conversation is too vague, produce a reasonable title from the clearest available intent.

Return JSON in this exact shape:

{
  "title": "Concise Title",
  "confidence": 0.0,
  "reason": "Brief reason for the title"
}
`

const titleGenerationUserPromptTemplate = `Generate a concise title for the following conversation.

Existing title:
%s

Title manually overridden:
%v

Mode:
%s

Conversation messages:
%s

Important:
- If mode is "create", generate the best initial title unless a manual title already exists.
- If mode is "regenerate", generate a fresh title suggestion even if an existing title is present.
- If mode is "refresh", only create a new title if the conversation has clearly shifted topics. Otherwise return the existing title.
- Keep the title short, specific, and useful in a chat sidebar.
- Do not expose sensitive details.
`

// GenerateConversationTitle is the reusable service/function for title generation.
func GenerateConversationTitle(ctx context.Context, llmProv llm.Provider, model string, input TitleGeneratorInput) (TitleGeneratorResponse, error) {
	// 1. Check for manual override or existing title in specific modes
	if input.TitleManuallyOverridden {
		if input.Mode == ModeCreate || input.Mode == ModeRefresh {
			return TitleGeneratorResponse{
				Title:      input.ExistingTitle,
				Confidence: 1.0,
				Reason:     "Preserved manually overridden title.",
			}, nil
		}
	}

	if llmProv == nil || strings.TrimSpace(model) == "" {
		return fallbackTitle(input), nil
	}

	// In create mode, if we already have a title, we might want to preserve it?
	// The requirement says: "If an existing manual title exists, preserve it."
	// We already handled manual override above. If it's not manually overridden but exists,
	// and mode is "create", we should probably still generate if it's empty or generic.
	// But usually "create" is called when there is no title.

	// 2. Prepare conversation context (first few meaningful messages)
	messagesContext := formatMessagesForPrompt(input.Messages)

	// 3. Render prompts
	existingTitleStr := "null"
	if input.ExistingTitle != "" {
		existingTitleStr = input.ExistingTitle
	}
	userPrompt := fmt.Sprintf(titleGenerationUserPromptTemplate,
		existingTitleStr,
		input.TitleManuallyOverridden,
		input.Mode,
		messagesContext,
	)

	// 4. Call LLM
	resp, err := llmProv.Chat(ctx, model, []llm.Message{
		{Role: "system", Content: titleGenerationSystemPrompt},
		{Role: "user", Content: userPrompt},
	}, nil, llm.Options{
		Temperature: 0.2,
		MaxTokens:   200,
	})

	if err != nil {
		return fallbackTitle(input), nil
	}

	// 5. Parse and validate response
	var titleResp TitleGeneratorResponse
	if err := json.Unmarshal([]byte(sanitizeJSON(resp.Content)), &titleResp); err != nil {
		return fallbackTitle(input), nil
	}

	// 6. Post-processing and sanitization
	titleResp.Title = cleanTitle(titleResp.Title)

	if !isValidTitle(titleResp.Title) {
		return fallbackTitle(input), nil
	}

	// Enforce length limit
	if len(titleResp.Title) > 100 {
		titleResp.Title = titleResp.Title[:100]
	}

	return titleResp, nil
}

func formatMessagesForPrompt(messages []llm.Message) string {
	var sb strings.Builder
	count := 0
	for _, m := range messages {
		if m.Role == "system" {
			continue
		}
		sb.WriteString(fmt.Sprintf("%s: %s\n", strings.ToUpper(m.Role), m.Content))
		count++
		if count >= 6 { // Only take the first few meaningful messages
			break
		}
	}
	return sb.String()
}

func sanitizeJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func cleanTitle(t string) string {
	t = strings.TrimSpace(t)
	// Remove surrounding quotes
	t = strings.Trim(t, "\"'")
	// Collapse repeated spaces
	re := regexp.MustCompile(`\s+`)
	t = re.ReplaceAllString(t, " ")
	return t
}

func isValidTitle(t string) bool {
	if t == "" {
		return false
	}
	tLower := strings.ToLower(t)
	genericTitles := []string{
		"chat summary",
		"ai discussion",
		"project help",
		"general question",
		"technology conversation",
		"new chat",
		"conversation",
	}
	for _, gt := range genericTitles {
		if tLower == gt {
			return false
		}
	}

	// Simple sensitive data check (regex for email, API keys, etc. can be added here)
	// For now, very basic check
	if strings.Contains(tLower, "password") || strings.Contains(tLower, "api_key") || strings.Contains(tLower, "secret") {
		return false
	}

	return true
}

func fallbackTitle(input TitleGeneratorInput) TitleGeneratorResponse {
	// 1. Try to extract from the first meaningful user message
	var firstUserMsg string
	for _, m := range input.Messages {
		if m.Role == "user" {
			firstUserMsg = m.Content
			break
		}
	}

	if firstUserMsg != "" {
		// Remove filler phrases
		fillers := []string{"can you help me with", "i want to", "how do i", "can you", "please"}
		t := strings.ToLower(firstUserMsg)
		for _, filler := range fillers {
			t = strings.ReplaceAll(t, filler, "")
		}
		t = strings.TrimSpace(t)
		if len(t) > 40 {
			t = t[:40]
		}
		// Convert to Title Case
		t = cases.Title(language.English).String(t)

		if isValidTitle(t) {
			return TitleGeneratorResponse{
				Title:      t,
				Confidence: 0.1,
				Reason:     "Fallback title extracted from first user message.",
			}
		}
	}

	return TitleGeneratorResponse{
		Title:      "New Conversation",
		Confidence: 0.1,
		Reason:     "Fallback title used because title generation failed.",
	}
}
