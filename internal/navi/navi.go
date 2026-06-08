// Package navi implements the always-on persistent AI agent, NAVI (Navi Navigator).
package navi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/artifact"
	"github.com/open-navi/navi/internal/blob"
	"github.com/open-navi/navi/internal/bus"
	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/experience"
	"github.com/open-navi/navi/internal/navi/orchestration"
	"github.com/open-navi/navi/internal/navi/proposals"
	"github.com/open-navi/navi/internal/navi/selfmod"
	"github.com/open-navi/navi/internal/navi/skill"
	"github.com/open-navi/navi/internal/presence"
	naviruntime "github.com/open-navi/navi/internal/runtime"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
	navitool "github.com/open-navi/navi/internal/tool"
)

// NAVI represents the main persistent agent.
type NAVI struct {
	cfg               Config
	chats             ChatStore
	runtimeSessions   RuntimeSessionStore
	runtime           naviruntime.Store
	skills            *skill.SkillRegistry
	experienceManager *ExperienceManager
	llm               llm.Provider
	bus               bus.Bus
	loop              *AgentLoop
	coordinator       *naviruntime.RunCoordinator
	intake            *MessageIntakeService
	endpointResolver  *EndpointResolver
	deliveryService   *DeliveryService
	orchestration     *orchestration.Pipeline
	skillBuilder      *skill.SkillBuilder
	status            *StatusTracker
	presence          presence.Service
	stopCoordinator   context.CancelFunc
}

var (
	ErrRuntimeStoreUnavailable  = errors.New("navi: runtime store not configured")
	ErrICSStateStoreUnavailable = errors.New("navi: ICS state store not configured")
)

// New initializes the NAVI agent component.
func New(cfg Config) (*NAVI, error) {
	profilesDir := strings.TrimSpace(cfg.ExperienceProfilesDir)
	initialMode := cfg.InitialExperienceMode
	em := NewExperienceManager(profilesDir, NormalizeExperienceMode(initialMode))
	if cfg.DB != nil {
		em.SetGovernanceBoundsProvider(experience.NewStoreGovernanceBoundsProvider(cfg.DB))
	}
	registry := skill.NewRegistry(cfg.SkillsDir)
	if cfg.PluginRegistry != nil {
		registry.SetPluginSkillSource(cfg.PluginRegistry)
	}
	policy := skill.NewPolicyEngine()

	if err := registry.Load(); err != nil {
		fmt.Printf("navi: starting with empty skills: %v\n", err)
	}

	if cfg.DB != nil && cfg.Cron != nil {
		skill.RegisterScheduleHandler(cfg.DB, cfg.Cron)
	}

	var hubClient *skill.HubClient
	if cfg.HubIndexURL != "" {
		hubClient = skill.NewHubClient(cfg.HubIndexURL)
	}

	statusTracker := NewStatusTracker()

	var artSvc *artifact.Service
	if cfg.DB != nil {
		artDir := strings.TrimSpace(cfg.ArtifactsDir)
		if artDir == "" {
			artDir = "artifacts"
		}
		blobStore, err := blob.NewFilesystemStore(artDir)
		if err != nil {
			fmt.Printf("navi: starting without artifact storage: %v\n", err)
		} else {
			artSvc = artifact.NewService(cfg.DB, blobStore)
			if cfg.Bus != nil {
				artSvc.SetEventPublisher(func(ctx context.Context, ev schema.Event) error {
					return cfg.Bus.Publish(ctx, ev)
				})
				artSvc.SetReflectionPublisher(func(ctx context.Context, payload schema.ReflectionPayload) error {
					ev := schema.NewEvent(schema.FactReflectionQueued, schema.EventKindFact, payload.ID, schema.AgentNavi, payload)
					ev.Visibility = schema.VisibilityOperator
					return cfg.Bus.Publish(ctx, ev)
				})
			}
			skill.RegisterArtifactHandler(cfg.DB, artSvc)
		}
	}

	chatStore := cfg.Chats
	if chatStore == nil {
		return nil, fmt.Errorf("navi: Chats (ChatStore) must be configured for conversational transcripts")
	}
	runtimeSessionStore := cfg.RuntimeSessions
	if runtimeSessionStore == nil {
		return nil, fmt.Errorf("navi: RuntimeSessions (RuntimeSessionStore) must be configured")
	}

	var runtimeStore naviruntime.Store
	if rs, ok := chatStore.(naviruntime.Store); ok {
		runtimeStore = rs
	} else if rs, ok := runtimeSessionStore.(naviruntime.Store); ok {
		runtimeStore = rs
	} else {
		return nil, fmt.Errorf("%w: a configured store must implement runtime.Store for conversational execution", ErrRuntimeStoreUnavailable)
	}

	endpointStore := cfg.ConversationEndpoints
	if endpointStore == nil {
		if es, ok := chatStore.(ConversationEndpointStore); ok {
			endpointStore = es
		}
	}
	var endpointResolver *EndpointResolver
	if endpointStore != nil {
		endpointResolver = &EndpointResolver{Chats: chatStore, Endpoints: endpointStore}
	}
	var deliveryService *DeliveryService
	if endpointStore != nil && cfg.ConnectorDispatcher != nil {
		deliveryService = &DeliveryService{Endpoints: endpointStore, Dispatcher: cfg.ConnectorDispatcher}
	}

	n := &NAVI{
		cfg:               cfg,
		chats:             chatStore,
		runtimeSessions:   runtimeSessionStore,
		experienceManager: em,
		llm:               cfg.LLM,
		bus:               cfg.Bus,
		runtime:           runtimeStore,
		skills:            registry,
		status:            statusTracker,
		endpointResolver:  endpointResolver,
		deliveryService:   deliveryService,
	}
	n.presence = presence.NewDefaultService(
		cfg.DB,
		statusTracker,
		presence.NewDBAttentionSource(cfg.DB),
		&presence.DefaultClassificationSource{},
		presence.NewDefaultHealthSource(statusTracker),
	)

	loopCfg := LoopConfig{
		NAVI:                             n,
		DB:                               cfg.DB,
		ExperienceManager:                em,
		Chats:                            chatStore,
		RuntimeSessions:                  runtimeSessionStore,
		Skills:                           registry,
		Policy:                           policy,
		LLM:                              cfg.LLM,
		Bus:                              cfg.Bus,
		Model:                            cfg.Model,
		LLMCallTimeout:                   normalizeLLMCallTimeout(cfg.LLMCallTimeout),
		Summarizer:                       cfg.Summarizer,
		WorkspaceDir:                     cfg.WorkspaceDir,
		Governor:                         cfg.Governor,
		WorldModel:                       cfg.WorldModel,
		Experience:                       experience.DefaultLayer{},
		StatusTracker:                    statusTracker,
		ListLLMs:                         cfg.ListLLMs,
		GetActiveLLM:                     cfg.GetActiveLLM,
		SetActiveLLM:                     cfg.SetActiveLLM,
		OnStartConnector:                 cfg.OnStartConnector,
		OnSetTimezone:                    cfg.OnSetTimezone,
		FactsBlock:                       cfg.FactsBlock,
		ContextRetriever:                 cfg.ContextRetriever,
		ContextRetrievalBudgetTokens:     cfg.ContextRetrievalBudgetTokens,
		LLMService:                       newLLMCallbacksRouter(cfg),
		SaveProposal:                     cfg.SaveProposal,
		GetProposal:                      cfg.GetProposal,
		FindPendingProposalByBoundaryKey: cfg.FindPendingProposalByBoundaryKey,
		ICSStateStore:                    cfg.ICSStateStore,
		SaveExecutionOutcome:             cfg.SaveExecutionOutcome,
		AutonomyResolver:                 cfg.AutonomyResolver,
		DomainForSkill:                   cfg.DomainForSkill,
		GovConfigPriorityReader:          cfg.GovConfigPriorityReader,
		ResolveOwnerID:                   cfg.ResolveOwnerID,
		ConnectorHealth:                  cfg.ConnectorHealth,
		PluginRegistry:                   cfg.PluginRegistry,
		SelfModExecutor:                  selfmod.NewExecutor(cfg.WorkspaceDir),
		ArtifactService:                  artSvc,
		Debug:                            cfg.Debug,
		RuntimeStore:                     runtimeStore,
		Cron:                             cfg.Cron,
		Prompts:                          cfg.Prompts,
	}

	if cfg.GapDetector != nil {
		loopCfg.GapDetector = cfg.GapDetector
	} else if cfg.DB != nil {
		loopCfg.GapDetector = NewGapDetector(cfg.DB)
	}

	if loopCfg.ICSStateStore == nil {
		if icsStore, ok := runtimeStore.(ICSStateStore); ok {
			loopCfg.ICSStateStore = icsStore
		} else if icsStore, ok := chatStore.(ICSStateStore); ok {
			loopCfg.ICSStateStore = icsStore
		}
	}
	if loopCfg.ICSStateStore == nil {
		return nil, fmt.Errorf("%w: configure Config.ICSStateStore or use a store implementing navi.ICSStateStore", ErrICSStateStoreUnavailable)
	}

	n.loop = NewAgentLoop(loopCfg)
	n.loop.cfg.NAVI = n
	if n.loop.cfg.Orchestration == nil {
		n.loop.cfg.Orchestration = newRuntimeOrchestrationPipeline(n.loop)
	}
	n.orchestration = n.loop.cfg.Orchestration

	if cfg.DB != nil {
		n.skillBuilder = skill.NewSkillBuilder(
			registry,
			cfg.LLM,
			cfg.Model,
			cfg.WorkspaceDir,
			cfg.DB,
			newSkillBuildGovernanceAdapter(n).Govern,
			hubClient,
		)
	}
	if cfg.RegisterPluginSkillHandlers != nil {
		cfg.RegisterPluginSkillHandlers(SkillHandlerContext{
			DB:             cfg.DB,
			LLM:            cfg.LLM,
			Model:          cfg.Model,
			WorkspaceDir:   cfg.WorkspaceDir,
			BraveSearchKey: cfg.BraveSearchKey,
			Hub:            hubClient,
			SkillBuilder:   n.skillBuilder,
			ResolveOwnerID: cfg.ResolveOwnerID,
		})
	}
	n.coordinator = naviruntime.NewRunCoordinator(runtimeStore, n.loop, cfg.SaveExecutionOutcome)
	n.coordinator.SetErrorRecorder(cfg.SaveErrorRecord)
	n.coordinator.SetRunTimeout(coordinatorRunTimeout(loopCfg.LLMCallTimeout))
	if deliveryService != nil {
		n.coordinator.SetAssistantMessageObserver(NewAssistantDeliveryObserver(deliveryService))
	}
	n.intake = &MessageIntakeService{
		Chats:            chatStore,
		RuntimeSessions:  runtimeSessionStore,
		Runtime:          n.coordinator,
		EndpointResolver: endpointResolver,
	}
	n.loop.cfg.RuntimeWake = n.coordinator.Wake

	return n, nil
}

// Skills returns the skill registry for operator/API use (e.g. GET /api/skills).
func (n *NAVI) Skills() *skill.SkillRegistry {
	return n.skills
}

// BuildExperienceControl derives the runtime experience-layer output using the
// active experience profiles loaded for this NAVI instance.
func (n *NAVI) BuildExperienceControl(ctx context.Context, mode ExperienceMode, req experience.BuildRequest) (experience.RenderedControl, error) {
	if n.experienceManager == nil {
		n.experienceManager = NewExperienceManager(n.cfg.ExperienceProfilesDir, NormalizeExperienceMode(n.cfg.InitialExperienceMode))
		n.experienceManager.SetGovernanceBoundsProvider(experience.NewStoreGovernanceBoundsProvider(n.cfg.DB))
	}
	return n.experienceManager.Build(ctx, NormalizeExperienceMode(mode), req)
}

// ToolRegistry returns the merged runtime tool registry (built lazily).
func (n *NAVI) ToolRegistry() *navitool.Registry {
	if n.loop == nil {
		return nil
	}
	reg, err := n.loop.EnsureToolRegistry()
	if err != nil {
		return nil
	}
	return reg
}

// ReloadSkills reloads skill definitions from disk and refreshes registered skill tools.
func (n *NAVI) ReloadSkills() error {
	if n.skills == nil {
		return errors.New("skill registry not configured")
	}
	if err := n.skills.Load(); err != nil {
		return err
	}
	if n.loop == nil {
		return nil
	}
	reg, err := n.loop.EnsureToolRegistry()
	if err != nil {
		return err
	}
	skillTools, err := newSkillToolRegistryAdapter(n.skills).Tools()
	if err != nil {
		return err
	}
	return reg.ReregisterSkills(skillTools)
}

// ReloadPrompts reloads on-disk auxiliary prompt templates (for example heartbeat templates)
// when a prompts.Manager is configured.
func (n *NAVI) ReloadPrompts() error {
	if n.cfg.Prompts != nil {
		return n.cfg.Prompts.Reload()
	}
	return nil
}

// Status returns a snapshot of agent activity for the gateway status API.
func (n *NAVI) Status() schema.AgentStatusSnapshot {
	if n.status == nil {
		return schema.AgentStatusSnapshot{State: schema.AgentStateOffline}
	}
	return n.status.Snapshot()
}

// StatusTracker returns the underlying tracker for the presence subsystem.
func (n *NAVI) StatusTracker() *StatusTracker {
	return n.status
}

func (n *NAVI) chatStore() ChatStore {
	if n == nil {
		return nil
	}
	return n.chats
}

func (n *NAVI) runtimeSessionStore() RuntimeSessionStore {
	if n == nil {
		return nil
	}
	return n.runtimeSessions
}

// PausedRun returns the current paused foreground run for a runtime session, if any.
func (n *NAVI) PausedRun(ctx context.Context, chatID string) (*naviruntime.RunState, error) {
	if n.runtime == nil || strings.TrimSpace(chatID) == "" {
		return nil, nil
	}
	return n.runtime.GetPausedRun(ctx, chatID)
}

// Start begins the runtime-session-scoped coordinator.
func (n *NAVI) Start(ctx context.Context) error {
	if n.coordinator != nil {
		cCtx, cancel := context.WithCancel(ctx)
		n.stopCoordinator = cancel
		go func() {
			_ = n.coordinator.Run(cCtx)
		}()
	}
	return nil
}

// Stop cleanly shuts down NAVI, draining in-flight runs before cancelling.
func (n *NAVI) Stop() error {
	if n.coordinator != nil {
		drainCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = n.coordinator.Drain(drainCtx)
	}
	if n.stopCoordinator != nil {
		n.stopCoordinator()
	}
	return nil
}

// CreateChat generates a new durable conversation thread.
func (n *NAVI) CreateChat(ctx context.Context, experienceMode ExperienceMode, projectID string) (string, error) {
	return n.CreateChatWithTitle(ctx, experienceMode, projectID, "New Chat")
}

// CreateChatWithTitle generates a new durable conversation thread with a caller-supplied title.
func (n *NAVI) CreateChatWithTitle(ctx context.Context, experienceMode ExperienceMode, projectID, title string) (string, error) {
	chats := n.chatStore()
	if chats == nil {
		return "", fmt.Errorf("navi: chat store is not configured")
	}
	scope := ActiveChatScope{ProjectID: strings.TrimSpace(projectID), SourceChannel: "app"}
	previousChatID, _ := chats.ActiveChatID(ctx, scope)
	mode := NormalizeExperienceMode(experienceMode)
	chat, err := chats.CreateChat(ctx, CreateChatInput{
		ProjectID:      strings.TrimSpace(projectID),
		ExperienceMode: string(mode),
		Title:          firstNonEmptyString(title, "New Chat"),
	})
	if err != nil {
		return "", err
	}
	if chat == nil {
		return "", nil
	}
	_ = chats.SetActiveChat(ctx, scope, string(chat.ID))
	if previousChatID != "" && previousChatID != string(chat.ID) && n.loop != nil && n.loop.cfg.Summarizer != nil {
		go func(previous string) {
			tCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if err := n.loop.cfg.Summarizer.SummarizeChat(tCtx, previous); err != nil {
				fmt.Printf("navi: summarize previous chat %s: %v\n", previous, err)
			}
		}(previousChatID)
	}
	return string(chat.ID), nil
}

// CreateCeremonyChat creates a new "Meet NAVI" chat thread with firstRunThread metadata.
func (n *NAVI) CreateCeremonyChat(ctx context.Context) (string, error) {
	chats := n.chatStore()
	if chats == nil {
		return "", fmt.Errorf("navi: chat store is not configured")
	}
	chat, err := chats.CreateChat(ctx, CreateChatInput{
		Title: "Meet NAVI",
		Metadata: map[string]any{
			"firstRunThread": map[string]any{
				"type":              "navi_ceremony",
				"subtitle":          "Your first conversation.",
				"ceremonyJourneyId": "navi_ceremony.v1",
			},
		},
	})
	if err != nil {
		return "", err
	}
	if chat == nil {
		return "", fmt.Errorf("navi: chat creation returned nil")
	}
	return string(chat.ID), nil
}

// InjectCeremonyMessage writes an assistant message directly into a chat without going through the LLM loop.
func (n *NAVI) InjectCeremonyMessage(ctx context.Context, chatID, content string, metadata map[string]any) (string, error) {
	chats := n.chatStore()
	if chats == nil {
		return "", fmt.Errorf("navi: chat store is not configured")
	}
	return chats.AppendChatMessage(ctx, chatID, ChatMessage{
		Role:     "assistant",
		Content:  content,
		Metadata: metadata,
	})
}

// RecordUserMessage writes a user message directly into a chat's transcript.
func (n *NAVI) RecordUserMessage(ctx context.Context, chatID, content string) (string, error) {
	chats := n.chatStore()
	if chats == nil {
		return "", fmt.Errorf("navi: chat store is not configured")
	}
	return chats.AppendChatMessage(ctx, chatID, ChatMessage{
		Role:    "user",
		Content: content,
	})
}

type proactiveMessageSessionStore interface {
	AppendSystemAssistantMessage(ctx context.Context, chatID, content, experienceMode, sourceChannel, kind string) (string, error)
}

// SendProactiveMessage appends a proactive assistant update to the best available owner-facing chat.
func (n *NAVI) SendProactiveMessage(ctx context.Context, content string) error {
	chats := n.chatStore()
	if chats == nil {
		return fmt.Errorf("navi: chat store is not configured")
	}
	store, ok := chats.(proactiveMessageSessionStore)
	if !ok {
		return fmt.Errorf("navi: chat store does not support proactive assistant messages")
	}
	chatID, experienceMode, err := n.resolveProactiveTargetChat(ctx)
	if err != nil {
		return err
	}
	if chatID == "" {
		return fmt.Errorf("navi: no open chat available for proactive message")
	}
	_, err = store.AppendSystemAssistantMessage(ctx, chatID, content, string(experienceMode), "heartbeat", string(schema.AssistantMessageKindProactive))
	return err
}

func (n *NAVI) resolveProactiveTargetChat(ctx context.Context) (string, ExperienceMode, error) {
	chats := n.chatStore()
	if chats == nil {
		return "", "", fmt.Errorf("navi: chat store is not configured")
	}
	scope := ActiveChatScope{SourceChannel: "app"}
	activeChatID, err := chats.ActiveChatID(ctx, scope)
	if err != nil {
		return "", "", err
	}
	if activeChatID != "" {
		if !isInternalRuntimeChatID(activeChatID) {
			if chat, err := chats.GetChat(ctx, activeChatID); err == nil && chat != nil {
				return string(chat.ID), ExperienceModeStandard, nil
			}
		}
	}
	recent, err := chats.ListChats(ctx, 10)
	if err != nil {
		return "", "", err
	}
	for _, chat := range recent {
		chatID := string(chat.ID)
		if chatID == "" || isInternalRuntimeChatID(chatID) {
			continue
		}
		return chatID, ExperienceModeStandard, nil
	}
	return "", "", nil
}

func isInternalRuntimeChatID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	if id == schema.HeartbeatAutoRuntimeSessionID || IsInternalRuntimeSessionID(id) {
		return true
	}
	return false
}

// RecentChats lists the most recently updated chats.
func (n *NAVI) RecentChats(ctx context.Context, limit int) ([]Chat, error) {
	chats := n.chatStore()
	if chats == nil {
		return nil, fmt.Errorf("navi: chat store is not configured")
	}
	return chats.ListChats(ctx, limit)
}

func (n *NAVI) RenameChat(ctx context.Context, chatID, title string) error {
	chats := n.chatStore()
	if chats == nil {
		return fmt.Errorf("navi: chat store is not configured")
	}
	return chats.RenameChat(ctx, chatID, title)
}

func (n *NAVI) UpdateChat(ctx context.Context, chat Chat) error {
	chats := n.chatStore()
	if chats == nil {
		return fmt.Errorf("navi: chat store is not configured")
	}
	return chats.UpdateChat(ctx, chat)
}

func (n *NAVI) ArchiveChat(ctx context.Context, chatID string) error {
	chats := n.chatStore()
	if chats == nil {
		return fmt.Errorf("navi: chat store is not configured")
	}
	return chats.ArchiveChat(ctx, chatID)
}

func (n *NAVI) DeleteChat(ctx context.Context, chatID string) error {
	chats := n.chatStore()
	if chats == nil {
		return fmt.Errorf("navi: chat store is not configured")
	}
	return chats.DeleteChat(ctx, chatID)
}

// RecentProjectChats lists recently updated chats for a single project.
func (n *NAVI) RecentProjectChats(ctx context.Context, projectID string, limit int) ([]Chat, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return n.RecentChats(ctx, limit)
	}
	chats := n.chatStore()
	if chats == nil {
		return nil, fmt.Errorf("navi: chat store is not configured")
	}
	return chats.ListChatsByProject(ctx, projectID, limit)
}

// GetChat returns a chat and its recent transcript messages.
func (n *NAVI) GetChat(ctx context.Context, chatID string) (*ChatThread, error) {
	chats := n.chatStore()
	if chats == nil {
		return nil, fmt.Errorf("navi: chat store is not configured")
	}
	return chats.GetChatWithMessages(ctx, chatID)
}

// chatFeedbackStore is the optional capability for persisting per-message owner
// feedback. The SQLite store implements it; other stores may not.
type chatFeedbackStore interface {
	SetChatMessageFeedback(ctx context.Context, chatID, messageID, rating string) error
}

// SetMessageFeedback records owner feedback (rating "up"/"down", or "" to clear)
// on a chat message. Returns an error if the configured store cannot persist it.
func (n *NAVI) SetMessageFeedback(ctx context.Context, chatID, messageID, rating string) error {
	chats := n.chatStore()
	if chats == nil {
		return fmt.Errorf("navi: chat store is not configured")
	}
	store, ok := chats.(chatFeedbackStore)
	if !ok {
		return fmt.Errorf("navi: chat store does not support feedback")
	}
	return store.SetChatMessageFeedback(ctx, chatID, messageID, rating)
}

// chatTruncateStore is the optional capability for truncating a thread from a
// given message onward (used by edit-and-resend).
type chatTruncateStore interface {
	DeleteChatMessagesFrom(ctx context.Context, chatID, messageID string) error
}

type chatVariantStore interface {
	EnsureMessageVariantGroup(ctx context.Context, chatID, messageID string) (string, error)
	ListMessageVariants(ctx context.Context, chatID, messageID string) (MessageVariants, error)
	SelectMessageVariant(ctx context.Context, chatID, messageID string, index int) error
}

// EditAndResendMessage edits a prior user message: it truncates the thread from
// that message onward and re-runs the turn with the new content. The edited
// message is re-appended (with a fresh id) via the normal intake path, so the
// runtime responds exactly as it would to a new message.
func (n *NAVI) EditAndResendMessage(ctx context.Context, chatID, messageID, newContent string) (*naviruntime.InboxItem, error) {
	content := strings.TrimSpace(newContent)
	if content == "" {
		return nil, fmt.Errorf("navi: edit-resend requires non-empty content")
	}
	chats := n.chatStore()
	if chats == nil {
		return nil, fmt.Errorf("navi: chat store is not configured")
	}
	truncator, ok := chats.(chatTruncateStore)
	if !ok {
		return nil, fmt.Errorf("navi: chat store does not support edit-and-resend")
	}

	// Verify the target is a user message before mutating anything.
	thread, err := chats.GetChatWithMessages(ctx, chatID)
	if err != nil {
		return nil, err
	}
	var target *ChatMessage
	for i := range thread.Messages {
		if string(thread.Messages[i].ID) == messageID {
			target = &thread.Messages[i]
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("navi: chat message not found")
	}
	if role := strings.ToLower(strings.TrimSpace(target.Role)); role != "user" && role != "owner" {
		return nil, fmt.Errorf("navi: only user messages can be edited and resent")
	}

	if err := truncator.DeleteChatMessagesFrom(ctx, chatID, messageID); err != nil {
		return nil, err
	}

	return n.SendMessageInput(ctx, chatID, naviruntime.MessageInput{
		Content:       content,
		SourceChannel: "app",
	})
}

// ContinueLastReply resumes from the most recent assistant reply without adding
// a fake user transcript row. The runtime receives a structured signal and
// appends the continuation as a follow-on assistant message.
func (n *NAVI) ContinueLastReply(ctx context.Context, chatID string) (*naviruntime.InboxItem, error) {
	chats := n.chatStore()
	if chats == nil {
		return nil, fmt.Errorf("navi: chat store is not configured")
	}
	thread, err := chats.GetChatWithMessages(ctx, chatID)
	if err != nil {
		return nil, err
	}
	for i := len(thread.Messages) - 1; i >= 0; i-- {
		msg := thread.Messages[i]
		if role := strings.ToLower(strings.TrimSpace(msg.Role)); role == "assistant" || role == "navi" {
			return n.queueChatActionSignal(ctx, chatID, "chat_continuation", map[string]string{
				"target_message_id": string(msg.ID),
				"assistant_prefix":  msg.Content,
			})
		}
	}
	return nil, fmt.Errorf("navi: no assistant message to continue")
}

// RegenerateLastReply re-runs the most recent user turn, producing a fresh
// assistant reply. The prior assistant reply is preserved as variant 0, and the
// runtime result replaces the visible content as the selected variant.
func (n *NAVI) RegenerateLastReply(ctx context.Context, chatID string) (*naviruntime.InboxItem, error) {
	chats := n.chatStore()
	if chats == nil {
		return nil, fmt.Errorf("navi: chat store is not configured")
	}
	variants, ok := chats.(chatVariantStore)
	if !ok {
		return nil, fmt.Errorf("navi: chat store does not support variants")
	}
	thread, err := chats.GetChatWithMessages(ctx, chatID)
	if err != nil {
		return nil, err
	}
	var targetAssistant *ChatMessage
	var lastUser *ChatMessage
	for i := len(thread.Messages) - 1; i >= 0; i-- {
		msg := thread.Messages[i]
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if targetAssistant == nil && (role == "assistant" || role == "navi") {
			targetAssistant = &thread.Messages[i]
			continue
		}
		if role == "user" || role == "owner" {
			lastUser = &thread.Messages[i]
			break
		}
	}
	if lastUser == nil {
		return nil, fmt.Errorf("navi: no user message to regenerate from")
	}
	if targetAssistant == nil {
		return nil, fmt.Errorf("navi: no assistant message to regenerate")
	}
	groupID, err := variants.EnsureMessageVariantGroup(ctx, chatID, string(targetAssistant.ID))
	if err != nil {
		return nil, err
	}
	return n.queueChatActionSignal(ctx, chatID, "chat_regenerate_variant", map[string]string{
		"target_message_id": string(targetAssistant.ID),
		"variant_group_id":  groupID,
		"user_message_id":   string(lastUser.ID),
		"user_content":      lastUser.Content,
	})
}

func (n *NAVI) ListMessageVariants(ctx context.Context, chatID, messageID string) (MessageVariants, error) {
	chats := n.chatStore()
	if chats == nil {
		return MessageVariants{}, fmt.Errorf("navi: chat store is not configured")
	}
	store, ok := chats.(chatVariantStore)
	if !ok {
		return MessageVariants{}, fmt.Errorf("navi: chat store does not support variants")
	}
	return store.ListMessageVariants(ctx, chatID, messageID)
}

func (n *NAVI) SelectMessageVariant(ctx context.Context, chatID, messageID string, index int) error {
	chats := n.chatStore()
	if chats == nil {
		return fmt.Errorf("navi: chat store is not configured")
	}
	store, ok := chats.(chatVariantStore)
	if !ok {
		return fmt.Errorf("navi: chat store does not support variants")
	}
	return store.SelectMessageVariant(ctx, chatID, messageID, index)
}

func (n *NAVI) queueChatActionSignal(ctx context.Context, chatID, payloadType string, payload map[string]string) (*naviruntime.InboxItem, error) {
	runtimeSessions := n.runtimeSessionStore()
	if runtimeSessions == nil {
		return nil, fmt.Errorf("navi: runtime session store is not configured")
	}
	runtimeSession, err := runtimeSessions.FindActiveRuntimeSessionForChat(ctx, chatID, "app")
	if err != nil {
		return nil, err
	}
	if runtimeSession == nil {
		runtimeSession, err = runtimeSessions.CreateRuntimeSession(ctx, CreateRuntimeSessionInput{
			Kind:           RuntimeSessionKindUser,
			ExperienceMode: string(NormalizeExperienceMode(n.experienceManager.Active().ID)),
			SourceChannel:  "app",
		})
		if err != nil {
			return nil, err
		}
		if err := runtimeSessions.AttachChatToRuntimeSession(ctx, string(runtimeSession.ID), chatID, RuntimeSessionChatPrimary); err != nil {
			return nil, err
		}
	}
	structured, _ := json.Marshal(payload)
	return n.SendSignal(ctx, &naviruntime.InboxItem{
		RuntimeSessionID: string(runtimeSession.ID),
		ChatID:           chatID,
		SourceChannel:    "app",
		ActorType:        "user",
		PayloadType:      payloadType,
		QueueAction:      "append",
		Status:           naviruntime.InboxStatusPending,
		Content:          firstNonEmptyString(payload["user_content"], payloadType),
		Structured:       structured,
		CorrelationID:    chatID,
	})
}

// SendMessage inserts a user message into a chat and queues the attached runtime session.
func (n *NAVI) SendMessage(ctx context.Context, chatID, content string) error {
	_, err := n.SendMessageInput(ctx, chatID, naviruntime.MessageInput{
		Content:       content,
		SourceChannel: "app",
	})
	return err
}

// SendMessageInput inserts a user message into the durable chat transcript and runtime inbox.
func (n *NAVI) SendMessageInput(ctx context.Context, chatID string, input naviruntime.MessageInput) (*naviruntime.InboxItem, error) {
	intake := n.intake
	if intake == nil && n.coordinator != nil {
		intake = &MessageIntakeService{
			Chats:            n.chatStore(),
			RuntimeSessions:  n.runtimeSessionStore(),
			Runtime:          n.coordinator,
			EndpointResolver: n.endpointResolver,
		}
	}
	if intake == nil {
		return nil, fmt.Errorf("%w: runtime coordinator is required; initialize NAVI with runtime store + ICS coordinator", ErrRuntimeCoordinatorUnavailable)
	}
	result, err := intake.SubmitMessage(ctx, SubmitMessageRequest{
		ChatID:           firstNonEmptyString(chatID, input.ChatID),
		Content:          input.Content,
		ExperienceMode:   string(n.experienceManager.Active().ID),
		SourceChannel:    input.SourceChannel,
		SourceMessageRef: input.SourceMessageRef,
		OriginEndpointID: input.OriginEndpointID,
		IdempotencyKey:   input.IdempotencyKey,
		Scope: ActiveChatScope{
			SourceChannel: input.SourceChannel,
		},
	})
	if err != nil {
		return nil, err
	}
	return result.InboxItem, nil
}

// SendSignal enqueues a structured runtime signal into the durable inbox.
func (n *NAVI) SendSignal(ctx context.Context, item *naviruntime.InboxItem) (*naviruntime.InboxItem, error) {
	if item == nil {
		return nil, fmt.Errorf("navi: send signal: nil inbox item")
	}
	if n.coordinator == nil {
		return nil, fmt.Errorf("navi: send signal: runtime not available")
	}
	return n.coordinator.SubmitSignal(ctx, item)
}

// SetExperienceMode updates the active experience-mode profile.
func (n *NAVI) SetExperienceMode(ctx context.Context, mode ExperienceMode) error {
	return n.experienceManager.Set(NormalizeExperienceMode(mode))
}

// ExecuteApprovedProposal is kept as a compatibility wrapper around runtime-aware proposal resolution.
func (n *NAVI) ExecuteApprovedProposal(ctx context.Context, proposalID string) (string, error) {
	if err := n.ResolveProposal(ctx, proposalID, schema.ProposalStatusApproved, schema.ResolutionTypeApprovedOnce, "owner", ""); err != nil {
		return "", err
	}
	return "Proposal approved; original run resumed.", nil
}

// EnrichActionWithIdentity adds chat-specific identity tags (e.g. owner status) to an action.
func (n *NAVI) EnrichActionWithIdentity(ctx context.Context, act *governor.ActionDescriptor) {
	if act == nil {
		return
	}
	var (
		isOwner     bool
		ownerChatID string
	)

	sourceChannel := ""
	sourceRef := ""
	if act.Tags != nil {
		sourceChannel = strings.TrimSpace(act.Tags["source_channel"])
		sourceRef = strings.TrimSpace(act.Tags["source_message_ref"])
	}

	// Connector-originated Telegram messages already carry the current chat ref,
	// so prefer that over inbox lookups on the ingress path.
	isTelegram := isTelegramSourceChannel(sourceChannel) ||
		(act.Domain == governor.AutonomyDomainMessaging && act.CommandType == schema.CommandTypeSend)
	if isTelegram {
		ownerChatID = n.isOwnerChatID(ctx)
		if sourceRef != "" && telegramRefMatchesChatID(sourceRef, ownerChatID) {
			isOwner = true
		}
	}

	// Handle chat/runtime-scoped governance enrichment.
	if !isOwner {
		isOwner = n.isOwnerChatOrRuntimeSession(ctx, act.ChatID)
	}

	if isOwner {
		if act.Tags == nil {
			act.Tags = make(map[string]string)
		}
		act.Tags["recipient_status"] = "owner"
	}

	// Messaging-specific channel recovery for when the action ID is a runtime
	// chat ID and we need to check the inbox correlation.
	if !isOwner {
		if isTelegram {
			// When an explicit source ref is present and did not match above, treat
			// it as authoritative and skip the extra inbox-origin lookup.
			if sourceRef == "" {
				// Fallback: Inbox check for the chat/runtime origin.
				var ref string
				// Query for the latest message in this chat/runtime scope to determine current owner context.
				if n.cfg.DB != nil {
					err := n.cfg.DB.QueryRowContext(ctx, `
						SELECT source_message_ref
						FROM navi_inbox
						WHERE chat_id = ? OR runtime_session_id = ?
						ORDER BY received_at DESC
						LIMIT 1
					`, act.ChatID, act.ChatID).Scan(&ref)
					if err == nil && telegramRefMatchesChatID(ref, ownerChatID) {
						isOwner = true
					}
				}
			}

			if isOwner {
				if act.Tags == nil {
					act.Tags = make(map[string]string)
				}
				act.Tags["recipient_status"] = "owner"
			}
		}
	}

	slog.Info("EnrichActionWithIdentity: enriched tags", "session", act.ChatID, "tags", act.Tags)
}

// ValidateAction evaluates whether a given action may execute by running the
// centralized governance pipeline and autonomy resolution.
func (n *NAVI) ValidateAction(ctx context.Context, act governor.ActionDescriptor) governor.ValidationResult {
	n.EnrichActionWithIdentity(ctx, &act)

	var v governor.ValidationResult
	if n.loop.cfg.GovConfigPriorityReader != nil {
		v = governor.ValidateActionWithOwner(ctx, n.loop.cfg.Policy, act, n.loop.cfg.GovConfigPriorityReader)
	} else {
		slog.Info("ValidateAction: owner session enriched", "session", act.ChatID, "tags", act.Tags)
		v = governor.ValidateAction(n.loop.cfg.Policy, act)
	}
	if n.loop.cfg.AutonomyResolver != nil {
		v = governor.ApplyAutonomyForAction(v, n.loop.cfg.AutonomyResolver, act.CommandType, act.Domain, act.SkillEntry)
	}
	return v
}

// ResolveProposal updates a proposal's status and resumes the original run for blocking session proposals.
func (n *NAVI) ResolveProposal(ctx context.Context, proposalID string, status schema.ProposalStatus, resolutionType schema.ResolutionType, resolvedBy, resolutionNote string) error {
	resolution := naviruntime.ProposalResolutionDecline
	if status == schema.ProposalStatusApproved {
		resolution = naviruntime.ProposalResolutionApprove
	}
	return n.resolveProposalWithResume(ctx, proposalID, status, resolutionType, resolvedBy, resolutionNote, resolution)
}

func (n *NAVI) ResolveBoundaryProposal(ctx context.Context, proposalID string, outcome schema.ApprovalOutcome, switchWorkspaceID, resolutionNote string) error {
	proposal, err := n.cfg.GetProposal(ctx, proposalID)
	if err != nil {
		return fmt.Errorf("get proposal: %w", err)
	}
	action, ok := proposals.ParseWorkspaceBoundaryAction(proposal)
	if !ok {
		return fmt.Errorf("proposal %s is not a workspace boundary proposal", proposalID)
	}

	if n.cfg.DB != nil {
		if err := store.UpdateExecutionOutcomesApprovalByProposalID(ctx, n.cfg.DB, proposalID, outcome, true); err != nil {
			return err
		}
	}

	switch outcome {
	case schema.ApprovalOutcomeDenied:
		return n.resolveProposalWithResume(ctx, proposalID, schema.ProposalStatusDeclined, schema.ResolutionTypeDenied, "owner", resolutionNote, naviruntime.ProposalResolutionDecline)
	case schema.ApprovalOutcomeAllowOnce:
		return n.resolveProposalWithResume(ctx, proposalID, schema.ProposalStatusApproved, schema.ResolutionTypeApprovedOnce, "owner", resolutionNote, naviruntime.ProposalResolutionApprove)
	case schema.ApprovalOutcomeAlwaysAllow:
		if n.cfg.WorldModel == nil {
			return errors.New("world model not configured")
		}
		workspaceID := strings.TrimSpace(action.WorkspaceID)
		if workspaceID == "" {
			workspaceID = n.cfg.WorldModel.GetActiveWorkspaceID(ctx)
		}
		if workspaceID == "" {
			return errors.New("workspace id required for always-allow boundary approval")
		}
		scope := firstNonEmpty(strings.TrimSpace(action.RuleScope), strings.TrimSpace(action.TargetPath))
		if scope == "" {
			return errors.New("boundary proposal missing rule scope")
		}
		rule := schema.WhitelistRule{
			RuleID:      uuid.New().String(),
			Scope:       scope,
			ActionTypes: normalizeBoundaryActionTypes(action.ActionTypes),
			Status:      schema.WhitelistRuleStatusActive,
			CreatedAt:   proposal.CreatedAt,
			CreatedBy:   "owner",
		}
		if err := n.cfg.WorldModel.UpsertWhitelistRule(ctx, rule, workspaceID); err != nil {
			return fmt.Errorf("save whitelist rule: %w", err)
		}
		return n.resolveProposalWithResume(ctx, proposalID, schema.ProposalStatusApproved, schema.ResolutionTypeApprovedAlways, "owner", resolutionNote, naviruntime.ProposalResolutionRevalidate)
	case schema.ApprovalOutcomeSwitchedWorkspace:
		if n.cfg.WorldModel == nil {
			return errors.New("world model not configured")
		}
		switchWorkspaceID = strings.TrimSpace(switchWorkspaceID)
		if switchWorkspaceID == "" {
			return errors.New("switch workspace id is required")
		}
		if err := n.cfg.WorldModel.SetActiveWorkspaceID(ctx, switchWorkspaceID); err != nil {
			return fmt.Errorf("set active workspace: %w", err)
		}
		return n.resolveProposalWithResume(ctx, proposalID, schema.ProposalStatusApproved, schema.ResolutionTypeSwitchedWorkspace, "owner", resolutionNote, naviruntime.ProposalResolutionRevalidate)
	default:
		return fmt.Errorf("unsupported boundary approval outcome: %s", outcome)
	}
}

func (n *NAVI) resolveProposalWithResume(ctx context.Context, proposalID string, status schema.ProposalStatus, resolutionType schema.ResolutionType, resolvedBy, resolutionNote string, resolution naviruntime.ProposalResolution) error {
	if n.cfg.GetProposal == nil || n.cfg.ResolveProposal == nil {
		return errors.New("proposal resolution not configured")
	}

	proposal, err := n.cfg.GetProposal(ctx, proposalID)
	if err != nil {
		return fmt.Errorf("get proposal: %w", err)
	}
	chatID, runID := proposalRuntimeContext(proposal)
	var pausedRun *naviruntime.RunState
	if chatID != "" && n.runtime != nil {
		if paused, loadErr := n.runtime.GetPausedRun(ctx, chatID); loadErr == nil && paused != nil {
			pausedRun = paused
			if runID == "" {
				runID = paused.RunID
			}
		}
	}

	resolved := false
	if proposal.Status == schema.ProposalStatusPending {
		if err := n.cfg.ResolveProposal(ctx, proposalID, status, resolutionType, resolvedBy, resolutionNote); err != nil {
			return err
		}
		resolved = true
	} else if proposal.Status != status {
		return fmt.Errorf("proposal not pending: %s", proposal.Status)
	}

	if chatID == "" || n.coordinator == nil || (status != schema.ProposalStatusApproved && status != schema.ProposalStatusDeclined) {
		return nil
	}
	if pausedRun != nil && pausedRun.BlockedOnProposalID != "" && pausedRun.BlockedOnProposalID != proposalID {
		return fmt.Errorf("paused run %s is blocked on proposal %s, not %s", pausedRun.RunID, pausedRun.BlockedOnProposalID, proposalID)
	}

	if resolved && n.runtime != nil {
		ev := schema.NewRunEvent(
			schema.FactProposalResolved,
			schema.EventKindFact,
			chatID,
			schema.AgentNavi,
			runID,
			schema.VisibilityUser,
			schema.ProposalResolvedPayload{
				RunID:            runID,
				RuntimeSessionID: chatID,
				ProposalID:       proposalID,
				Status:           string(status),
				ResolvedBy:       resolvedBy,
				Note:             resolutionNote,
			},
		)
		if err := n.runtime.AppendRuntimeEvent(ctx, ev); err != nil {
			return fmt.Errorf("append proposal resolved event: %w", err)
		}
	}

	_, err = n.coordinator.EnqueueProposalResolution(ctx, chatID, proposalID, resolution, resolutionNote)
	return err
}

// BuildGap runs gap expansion (skill, connector, etc.) via the skill builder.
func (n *NAVI) BuildGap(ctx context.Context, gapID string, userContext string, chatID string) (*skill.BuildResult, error) {
	if n.skillBuilder == nil {
		return nil, errors.New("skill builder not configured")
	}
	gap, err := store.GetGap(ctx, n.cfg.DB, gapID)
	if err != nil {
		return nil, err
	}
	if gap == nil {
		return nil, fmt.Errorf("gap %s not found", gapID)
	}
	req := skill.BuildRequest{
		Gap:         *gap,
		UserContext: userContext,
		ChatID:      chatID,
	}
	result, err := n.skillBuilder.Build(ctx, req)
	if err != nil {
		return nil, err
	}
	if result != nil && result.Kind == "connector" && result.ConnectorName != "" && n.cfg.OnStartConnector != nil {
		if err := n.cfg.OnStartConnector(ctx, result.ConnectorName, nil); err != nil {
			result.ActivationError = fmt.Sprintf("activate generated connector %s: %v", result.ConnectorName, err)
		}
	}
	return result, nil
}

// BuildSkill is an alias for BuildGap.
func (n *NAVI) BuildSkill(ctx context.Context, gapID string, userContext string, chatID string) (*skill.BuildResult, error) {
	return n.BuildGap(ctx, gapID, userContext, chatID)
}

// CancelRun cancels the active foreground run for a runtime session.
func (n *NAVI) CancelRun(chatID string) error {
	if n.coordinator == nil {
		return errors.New("run coordinator not configured")
	}
	return n.coordinator.CancelRun(chatID)
}

// ResumeRun wakes the coordinator for a runtime session.
func (n *NAVI) ResumeRun(chatID string) error {
	if n.coordinator == nil {
		return errors.New("run coordinator not configured")
	}
	n.coordinator.ResumeRun(chatID)
	return nil
}

func proposalRuntimeContext(proposal schema.Proposal) (chatID, runID string) {
	var action struct {
		ChatID string `json:"chat_id"`
		RunID  string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(proposal.ProposedAction), &action); err == nil {
		return action.ChatID, action.RunID
	}
	return "", ""
}

func normalizeBoundaryActionTypes(actionTypes []string) []string {
	if len(actionTypes) == 0 {
		return []string{"all"}
	}
	seen := make(map[string]struct{}, len(actionTypes))
	out := make([]string, 0, len(actionTypes))
	for _, raw := range actionTypes {
		value := strings.ToLower(strings.TrimSpace(raw))
		if value == "" {
			continue
		}
		if value != "all" {
			if _, err := schema.ParseWorkspaceActionType(value); err != nil {
				continue
			}
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return []string{"all"}
	}
	return out
}

// IsSetupComplete checks if the initial onboarding has been finished.
// Prefers the persistent IsSetupDone callback (settings table) when available.
func (n *NAVI) IsSetupComplete(ctx context.Context) bool {
	if n.cfg.IsSetupDone != nil {
		return n.cfg.IsSetupDone(ctx)
	}
	if n.cfg.DB != nil {
		complete, _, _ := store.GetSetting(ctx, n.cfg.DB, "setup_complete")
		return complete == "true"
	}
	return false
}

func (n *NAVI) isOwnerChatOrRuntimeSession(ctx context.Context, id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	// Internal runtime sessions (heartbeats, etc.) are always owner-controlled.
	if IsInternalRuntimeSessionID(id) {
		return true
	}
	// Check against the primary owner chat ID from configuration.
	if n.cfg.DB != nil {
		ownerChatID, _, _ := store.GetSetting(ctx, n.cfg.DB, "telegram_owner_chat_id")
		if ownerChatID != "" {
			if ownerChatID == id {
				return true
			}
			// Fallback: check if the inbox item was originally created from the owner's channel.
			var channel, ref string
			err := n.cfg.DB.QueryRowContext(ctx, `
				SELECT source_channel, source_message_ref
				FROM navi_inbox
				WHERE (chat_id = ? OR runtime_session_id = ?)
				  AND source_channel = 'telegram'
				LIMIT 1
			`, id, id).Scan(&channel, &ref)
			if err == nil && telegramRefMatchesChatID(ref, ownerChatID) {
				return true
			}
		}
	}
	return false
}

func (n *NAVI) isOwnerChatID(ctx context.Context) string {
	if n.cfg.DB != nil {
		ownerChatID, _, _ := store.GetSetting(ctx, n.cfg.DB, "telegram_owner_chat_id")
		return ownerChatID
	}
	return ""
}

func isTelegramSourceChannel(source string) bool {
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		return false
	}
	return source == "telegram" || strings.HasPrefix(source, "telegram-")
}

// telegramRefMatchesChatID checks whether a Telegram source_message_ref
// (format "chatID:threadID:messageID" or bare "chatID") originated from
// the given chat ID.
func telegramRefMatchesChatID(ref, chatID string) bool {
	ref = strings.TrimSpace(ref)
	chatID = strings.TrimSpace(chatID)
	if ref == "" || chatID == "" {
		return false
	}
	// Direct match covers the bare chat ID case.
	if ref == chatID {
		return true
	}
	// Composite format: extract the leading segment before the first colon.
	if idx := strings.IndexByte(ref, ':'); idx > 0 {
		return ref[:idx] == chatID
	}
	return false
}
