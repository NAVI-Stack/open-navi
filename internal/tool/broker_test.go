package tool

import (
	"testing"

	"github.com/ceoai/navi/internal/schema"
)

func TestToolBrokerResolveSmalltalkDefaultsToNoTools(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(validTestTool("navi.files.read")); err != nil {
		t.Fatalf("register read tool: %v", err)
	}

	result := NewToolBrokerFromRegistry(reg).Resolve(BrokerInput{
		UserInput:       "yo",
		SessionMode:     DiscoverySessionModeAssistant,
		Environment:     "production",
		UserAuthority:   ToolAuthorityOwner,
		ActiveToolSetID: "ats-001",
		ActiveToolIDs:   []string{"navi.files.read"},
		ModelProfile: BrokerModelProfile{
			Name:             "frontier_strong",
			SupportsTools:    true,
			ToolCallReliable: true,
		},
	})

	if result.ToolChoiceMode != ToolChoiceModeNone {
		t.Fatalf("expected tool choice none, got %s", result.ToolChoiceMode)
	}
	if len(result.SelectedToolIDs) != 0 {
		t.Fatalf("expected no selected tools, got %+v", result.SelectedToolIDs)
	}
	if result.Intent != BrokerIntentSmalltalk {
		t.Fatalf("expected smalltalk intent, got %s", result.Intent)
	}
	if len(result.SuppressedTools) != 1 || result.SuppressedTools[0].Reason != BrokerSuppressionSmalltalkNoTools {
		t.Fatalf("expected cached tool suppression for smalltalk, got %+v", result.SuppressedTools)
	}
}

func TestToolBrokerResolveConversationalTurnsKeepMinimalSafeSurface(t *testing.T) {
	reg := NewRegistry()

	chatSafe := validTestTool("navi.user.profile_lookup")
	chatSafe.DisplayName = "Profile Lookup"
	chatSafe.Description = "Look up durable user profile details."
	chatSafe.Aliases = []string{"profile lookup"}
	chatSafe.CapabilityTags = []string{"profile", "user"}
	chatSafe.Category = ToolCategoryChatSafe
	chatSafe.EnvironmentVisibility = []string{"production", "development"}
	if err := reg.Register(chatSafe); err != nil {
		t.Fatalf("register chat-safe tool: %v", err)
	}

	workflow := validTestTool("navi.messaging.send_reply")
	workflow.DisplayName = "Send Reply"
	workflow.Description = "Send a workflow reply."
	workflow.Aliases = []string{"send reply"}
	workflow.CapabilityTags = []string{"message", "reply"}
	workflow.Category = ToolCategoryWorkflowAction
	workflow.EnvironmentVisibility = []string{"production", "development"}
	if err := reg.Register(workflow); err != nil {
		t.Fatalf("register workflow tool: %v", err)
	}

	devTool := validTestTool("navi.dev.workspace_probe")
	devTool.DisplayName = "Workspace Probe"
	devTool.Description = "Inspect the development workspace."
	devTool.Aliases = []string{"workspace probe"}
	devTool.CapabilityTags = []string{"workspace", "probe", "dev"}
	devTool.Category = ToolCategoryDevTest
	devTool.Metadata.ExposureClass = ToolExposureDevelopment
	devTool.EnvironmentVisibility = []string{"development", "test"}
	if err := reg.Register(devTool); err != nil {
		t.Fatalf("register dev tool: %v", err)
	}

	internalTool := validTestTool("navi.debug.session_trace")
	internalTool.DisplayName = "Session Trace"
	internalTool.Description = "Inspect internal session traces."
	internalTool.Aliases = []string{"session trace"}
	internalTool.CapabilityTags = []string{"diagnostic", "trace"}
	internalTool.Category = ToolCategoryInternalDiagnostic
	internalTool.Metadata.ExposureClass = ToolExposureInternal
	internalTool.EnvironmentVisibility = []string{"production", "development"}
	if err := reg.Register(internalTool); err != nil {
		t.Fatalf("register internal tool: %v", err)
	}

	broker := NewToolBrokerFromRegistry(reg)
	baseInput := BrokerInput{
		SessionMode:   DiscoverySessionModeAssistant,
		Environment:   "production",
		UserAuthority: ToolAuthorityOwner,
		ModelProfile: BrokerModelProfile{
			Name:             "frontier_strong",
			SupportsTools:    true,
			ToolCallReliable: true,
		},
	}

	tests := []struct {
		name       string
		input      string
		wantIntent BrokerIntent
	}{
		{name: "greeting", input: "hello there", wantIntent: BrokerIntentSmalltalk},
		{name: "acknowledgment", input: "thanks, that helps", wantIntent: BrokerIntentSmalltalk},
		{name: "brief_follow_up", input: "that makes sense", wantIntent: BrokerIntentAmbiguous},
		{name: "casual_reply_without_task_intent", input: "sounds good to me", wantIntent: BrokerIntentAmbiguous},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := broker.Resolve(BrokerInput{
				UserInput:     tc.input,
				SessionMode:   baseInput.SessionMode,
				Environment:   baseInput.Environment,
				UserAuthority: baseInput.UserAuthority,
				ModelProfile:  baseInput.ModelProfile,
				ActiveToolIDs: []string{
					chatSafe.ToolID,
					workflow.ToolID,
					devTool.ToolID,
					internalTool.ToolID,
				},
			})

			if result.Intent != tc.wantIntent {
				t.Fatalf("intent = %s, want %s", result.Intent, tc.wantIntent)
			}
			if result.ToolChoiceMode != ToolChoiceModeNone {
				t.Fatalf("expected no tool surface for casual turn, got %s", result.ToolChoiceMode)
			}
			if len(result.SelectedToolIDs) != 0 {
				t.Fatalf("expected no selected tools for casual turn, got %+v", result.SelectedToolIDs)
			}
			if containsString(result.SelectedToolIDs, workflow.ToolID) ||
				containsString(result.SelectedToolIDs, devTool.ToolID) ||
				containsString(result.SelectedToolIDs, internalTool.ToolID) {
				t.Fatalf("casual turn leaked non-minimal surfaces: %+v", result.SelectedToolIDs)
			}

			wantSuppression := BrokerSuppressionAmbiguousRequest
			if tc.wantIntent == BrokerIntentSmalltalk {
				wantSuppression = BrokerSuppressionSmalltalkNoTools
			}
			for _, toolID := range []string{
				chatSafe.ToolID,
				workflow.ToolID,
				devTool.ToolID,
				internalTool.ToolID,
			} {
				if !hasSuppressedTool(result.SuppressedTools, toolID, wantSuppression) {
					t.Fatalf("expected suppression %s for %s, got %+v", wantSuppression, toolID, result.SuppressedTools)
				}
			}
		})
	}
}

func TestToolBrokerResolveAmbiguousRequestPrefersNoTools(t *testing.T) {
	reg := NewRegistry()
	searchTool := validTestTool("navi.repo.search")
	searchTool.DisplayName = "Repo Search"
	searchTool.Description = "Search the repository for code and files."
	searchTool.Aliases = []string{"repo search"}
	searchTool.CapabilityTags = []string{"repo", "search", "code"}
	searchTool.Metadata.ExposureClass = ToolExposureDevelopment
	searchTool.EnvironmentVisibility = []string{"development", "test"}
	if err := reg.Register(searchTool); err != nil {
		t.Fatalf("register search tool: %v", err)
	}

	result := NewToolBrokerFromRegistry(reg).Resolve(BrokerInput{
		UserInput:     "can you handle that?",
		SessionMode:   DiscoverySessionModeAssistant,
		Environment:   "production",
		UserAuthority: ToolAuthorityOwner,
		ModelProfile: BrokerModelProfile{
			Name:             "frontier_strong",
			SupportsTools:    true,
			ToolCallReliable: true,
		},
	})

	if result.ToolChoiceMode != ToolChoiceModeNone {
		t.Fatalf("expected ambiguous request to choose none, got %s", result.ToolChoiceMode)
	}
	if len(result.SelectedToolIDs) != 0 {
		t.Fatalf("expected no selected tools for ambiguous request, got %+v", result.SelectedToolIDs)
	}
	if result.Intent != BrokerIntentAmbiguous {
		t.Fatalf("expected ambiguous intent, got %s", result.Intent)
	}
}

func TestToolBrokerResolveCodingQueryNarrowsCandidateSet(t *testing.T) {
	reg := NewRegistry()

	searchTool := validTestTool("navi.repo.search")
	searchTool.DisplayName = "Repo Search"
	searchTool.Description = "Search the repository for code, symbols, and strings."
	searchTool.Aliases = []string{"repo search", "search code"}
	searchTool.CapabilityTags = []string{"repo", "search", "code"}
	searchTool.Category = ToolCategoryReadOnly
	searchTool.Metadata.ExposureClass = ToolExposureDevelopment
	searchTool.EnvironmentVisibility = []string{"development", "test"}
	searchTool.RequiredModes = []schema.DirectiveMode{schema.DirectiveModeAct}
	if err := reg.Register(searchTool); err != nil {
		t.Fatalf("register search tool: %v", err)
	}

	readTool := validTestTool("navi.files.read")
	readTool.DisplayName = "Read File"
	readTool.Description = "Read a file from the workspace."
	readTool.Aliases = []string{"read file", "open file"}
	readTool.CapabilityTags = []string{"file", "files", "workspace", "read"}
	readTool.Category = ToolCategoryReadOnly
	readTool.Metadata.ExposureClass = ToolExposureDevelopment
	readTool.EnvironmentVisibility = []string{"development", "test"}
	readTool.RequiredModes = []schema.DirectiveMode{schema.DirectiveModeAct}
	if err := reg.Register(readTool); err != nil {
		t.Fatalf("register read tool: %v", err)
	}

	diffTool := validTestTool("navi.git.diff")
	diffTool.DisplayName = "Git Diff"
	diffTool.Description = "Inspect git diffs and changed code."
	diffTool.Aliases = []string{"git diff"}
	diffTool.CapabilityTags = []string{"git", "diff", "code"}
	diffTool.Category = ToolCategoryReadOnly
	diffTool.Metadata.ExposureClass = ToolExposureDevelopment
	diffTool.EnvironmentVisibility = []string{"development", "test"}
	diffTool.RequiredModes = []schema.DirectiveMode{schema.DirectiveModeAct}
	if err := reg.Register(diffTool); err != nil {
		t.Fatalf("register diff tool: %v", err)
	}

	calendarTool := validTestTool("navi.calendar.lookup")
	calendarTool.DisplayName = "Calendar Lookup"
	calendarTool.Description = "Read the current calendar."
	calendarTool.Aliases = []string{"calendar"}
	calendarTool.CapabilityTags = []string{"calendar", "schedule"}
	if err := reg.Register(calendarTool); err != nil {
		t.Fatalf("register calendar tool: %v", err)
	}

	result := NewToolBrokerFromRegistry(reg).Resolve(BrokerInput{
		UserInput:     "search the repo for the duplicated thinking placeholder",
		SessionMode:   DiscoverySessionModeCoder,
		Environment:   "development",
		UserAuthority: ToolAuthorityOwner,
		ModelProfile: BrokerModelProfile{
			Name:             "local_weak",
			SupportsTools:    true,
			ToolCallReliable: true,
		},
	})

	if result.ToolChoiceMode != ToolChoiceModeAllowedSubset {
		t.Fatalf("expected allowed_subset for narrowed local coder set, got %s", result.ToolChoiceMode)
	}
	if len(result.SelectedToolIDs) == 0 || len(result.SelectedToolIDs) > 2 {
		t.Fatalf("expected narrow selected tool set, got %+v", result.SelectedToolIDs)
	}
	if containsString(result.SelectedToolIDs, "navi.calendar.lookup") {
		t.Fatalf("unexpected unrelated calendar tool in coder set: %+v", result.SelectedToolIDs)
	}
	if !containsString(result.SelectedToolIDs, "navi.repo.search") {
		t.Fatalf("expected repo search in coder tool set, got %+v", result.SelectedToolIDs)
	}
	if !hasSuppressedTool(result.SuppressedTools, "navi.git.diff", BrokerSuppressionLocalModelBudget) &&
		!hasSuppressedTool(result.SuppressedTools, "navi.files.read", BrokerSuppressionLocalModelBudget) {
		t.Fatalf("expected local model budget suppression, got %+v", result.SuppressedTools)
	}
}

func TestToolBrokerResolveDiagnosticRequestSuppressesDebugToolsInCompanionProduction(t *testing.T) {
	reg := NewRegistry()
	traceTool := validTestTool("navi.debug.session_trace")
	traceTool.DisplayName = "Session Trace"
	traceTool.Description = "Inspect debug traces and duplicated session events."
	traceTool.Aliases = []string{"session trace", "debug trace"}
	traceTool.CapabilityTags = []string{"diagnostic", "debug", "trace", "session"}
	traceTool.Category = ToolCategoryInternalDiagnostic
	traceTool.Metadata.ExposureClass = ToolExposureInternal
	traceTool.EnvironmentVisibility = []string{"development", "production"}
	traceTool.RequiredModes = []schema.DirectiveMode{schema.DirectiveModeAct}
	if err := reg.Register(traceTool); err != nil {
		t.Fatalf("register trace tool: %v", err)
	}

	result := NewToolBrokerFromRegistry(reg).Resolve(BrokerInput{
		UserInput:     "Can you check why the last Telegram message duplicated thinking bubbles?",
		Intent:        BrokerIntentDiagnosticRequest,
		SessionMode:   DiscoverySessionModeCompanion,
		Environment:   "production",
		UserAuthority: ToolAuthorityOwner,
		ModelProfile: BrokerModelProfile{
			Name:             "frontier_strong",
			SupportsTools:    true,
			ToolCallReliable: true,
		},
	})

	if result.ToolChoiceMode != ToolChoiceModeNone {
		t.Fatalf("expected no tool exposure, got %s", result.ToolChoiceMode)
	}
	if len(result.SelectedToolIDs) != 0 {
		t.Fatalf("expected no selected tools, got %+v", result.SelectedToolIDs)
	}
	if !hasSuppressedTool(result.SuppressedTools, "navi.debug.session_trace", BrokerSuppressionRequiresDebugMode) {
		t.Fatalf("expected debug suppression, got %+v", result.SuppressedTools)
	}
}

func TestToolBrokerResolveAssistantTaskSelectsTargetedUserFacingTool(t *testing.T) {
	reg := NewRegistry()
	calendarTool := validTestTool("navi.calendar.read_availability")
	calendarTool.DisplayName = "Calendar Availability"
	calendarTool.Description = "Check calendar availability and schedule windows."
	calendarTool.Aliases = []string{"calendar availability", "check calendar"}
	calendarTool.CapabilityTags = []string{"calendar", "availability", "schedule"}
	calendarTool.Governance.Domain = "calendar"
	calendarTool.EnvironmentVisibility = []string{"production", "development"}
	if err := reg.Register(calendarTool); err != nil {
		t.Fatalf("register calendar tool: %v", err)
	}

	replyTool := validTestTool("navi.messaging.send_reply")
	replyTool.DisplayName = "Send Reply"
	replyTool.Description = "Send a reply message to the user."
	replyTool.Aliases = []string{"send reply"}
	replyTool.CapabilityTags = []string{"message", "reply"}
	replyTool.Category = ToolCategoryWorkflowAction
	replyTool.EnvironmentVisibility = []string{"production", "development"}
	if err := reg.Register(replyTool); err != nil {
		t.Fatalf("register reply tool: %v", err)
	}

	result := NewToolBrokerFromRegistry(reg).Resolve(BrokerInput{
		UserInput:     "check my calendar availability this afternoon",
		SessionMode:   DiscoverySessionModeAssistant,
		Environment:   "production",
		UserAuthority: ToolAuthorityOwner,
		ModelProfile: BrokerModelProfile{
			Name:             "frontier_strong",
			SupportsTools:    true,
			ToolCallReliable: true,
		},
	})

	if len(result.SelectedToolIDs) != 1 || result.SelectedToolIDs[0] != "navi.calendar.read_availability" {
		t.Fatalf("expected one targeted calendar tool, got %+v", result.SelectedToolIDs)
	}
	if result.ToolChoiceMode != ToolChoiceModeForcedSingle {
		t.Fatalf("expected forced_single for targeted assistant task, got %s", result.ToolChoiceMode)
	}
	if containsString(result.SelectedToolIDs, "navi.messaging.send_reply") {
		t.Fatalf("unexpected reply tool in calendar task: %+v", result.SelectedToolIDs)
	}
	if result.Trace.SelectionLimit != 5 {
		t.Fatalf("expected strong-model selection limit in trace, got %+v", result.Trace)
	}
	if len(result.Trace.RankedCandidates) == 0 {
		t.Fatalf("expected ranked candidates trace, got %+v", result.Trace)
	}
	if !result.Trace.RankedCandidates[0].Selected {
		t.Fatalf("expected selected trace candidate, got %+v", result.Trace.RankedCandidates[0])
	}
}

func TestToolBrokerResolveStrongModelSelectsMoreToolsThanLocalWeak(t *testing.T) {
	reg := NewRegistry()
	for _, toolEntry := range []*Tool{
		coderTool("navi.repo.search", "Repo Search", "Search the repo for code and symbols.", "repo", "search", "code"),
		coderTool("navi.files.read", "Read File", "Read a workspace file.", "files", "workspace", "read"),
		coderTool("navi.git.diff", "Git Diff", "Inspect git diff output.", "git", "diff", "code"),
		coderTool("navi.test.logs", "Test Logs", "Inspect test logs and output.", "tests", "logs", "output"),
		coderTool("navi.workspace.scan", "Workspace Scan", "Scan the workspace for matching files.", "workspace", "scan", "files"),
	} {
		if err := reg.Register(toolEntry); err != nil {
			t.Fatalf("register tool %q: %v", toolEntry.ToolID, err)
		}
	}

	broker := NewToolBrokerFromRegistry(reg)
	input := BrokerInput{
		UserInput:     "inspect the repo files diff and test logs for the duplicated placeholder",
		SessionMode:   DiscoverySessionModeCoder,
		Environment:   "development",
		UserAuthority: ToolAuthorityOwner,
	}

	local := broker.Resolve(withModel(input, "local_weak"))
	strong := broker.Resolve(withModel(input, "frontier_strong"))

	if len(local.SelectedToolIDs) >= len(strong.SelectedToolIDs) {
		t.Fatalf("expected strong model to expose more tools than local weak, local=%+v strong=%+v", local.SelectedToolIDs, strong.SelectedToolIDs)
	}
	if local.Trace.SelectionLimit >= strong.Trace.SelectionLimit {
		t.Fatalf("expected stronger model trace limit to be higher, local=%+v strong=%+v", local.Trace, strong.Trace)
	}
}

func TestBrokerInputNormalizeDefaultsToProductionAssistantUser(t *testing.T) {
	normalized := (BrokerInput{}).normalize()
	if normalized.Environment != "production" {
		t.Fatalf("expected missing environment to default to production, got %+v", normalized)
	}
	if normalized.SessionMode != DiscoverySessionModeAssistant {
		t.Fatalf("expected missing session mode to default to assistant, got %+v", normalized)
	}
	if normalized.UserAuthority != ToolAuthorityUser {
		t.Fatalf("expected missing authority to default to user, got %+v", normalized)
	}
}

func TestToolBrokerResolveMissingContextDoesNotWidenDevelopmentTools(t *testing.T) {
	reg := NewRegistry()
	devTool := validTestTool("navi.dev.workspace_probe")
	devTool.DisplayName = "Workspace Probe"
	devTool.Description = "Probe the development workspace."
	devTool.Aliases = []string{"workspace probe"}
	devTool.CapabilityTags = []string{"workspace", "probe", "dev"}
	devTool.Metadata.ExposureClass = ToolExposureDevelopment
	devTool.Category = ToolCategoryDevTest
	devTool.EnvironmentVisibility = []string{"development", "test"}
	if err := reg.Register(devTool); err != nil {
		t.Fatalf("register dev tool: %v", err)
	}

	result := NewToolBrokerFromRegistry(reg).Resolve(BrokerInput{
		UserInput: "run the workspace probe",
		ModelProfile: BrokerModelProfile{
			Name:             "frontier_strong",
			SupportsTools:    true,
			ToolCallReliable: true,
		},
	})

	if len(result.SelectedToolIDs) != 0 {
		t.Fatalf("expected omitted context to fail closed on dev tool, got %+v", result.SelectedToolIDs)
	}
	if result.ToolChoiceMode != ToolChoiceModeNone {
		t.Fatalf("expected omitted context to keep tool choice at none, got %+v", result)
	}
}

func TestToolBrokerResolveConnectorDependenciesRequireAvailableConnector(t *testing.T) {
	reg := NewRegistry()
	connectorTool := validTestTool("navi.linear.create_issue")
	connectorTool.DisplayName = "Create Linear Issue"
	connectorTool.Description = "Create Linear issues from assistant turns."
	connectorTool.Aliases = []string{"create linear issue", "linear ticket"}
	connectorTool.CapabilityTags = []string{"linear", "issue", "ticket"}
	connectorTool.Governance.Domain = "linear"
	connectorTool.EnvironmentVisibility = []string{"production", "development"}
	connectorTool.ConnectorDependencies = []string{"linear"}
	if err := reg.Register(connectorTool); err != nil {
		t.Fatalf("register connector tool: %v", err)
	}

	broker := NewToolBrokerFromRegistry(reg)
	baseInput := BrokerInput{
		UserInput:     "create a linear ticket for the onboarding bug",
		SessionMode:   DiscoverySessionModeAssistant,
		Environment:   "production",
		UserAuthority: ToolAuthorityOwner,
		ModelProfile: BrokerModelProfile{
			Name:             "frontier_strong",
			SupportsTools:    true,
			ToolCallReliable: true,
		},
	}

	missing := broker.Resolve(baseInput)
	if len(missing.SelectedToolIDs) != 0 {
		t.Fatalf("expected connector-backed tool to stay suppressed without dependency, got %+v", missing.SelectedToolIDs)
	}
	if !hasSuppressedTool(missing.SuppressedTools, "navi.linear.create_issue", BrokerSuppressionConnectorUnavailable) {
		t.Fatalf("expected connector_unavailable suppression, got %+v", missing.SuppressedTools)
	}

	available := broker.Resolve(BrokerInput{
		UserInput:           baseInput.UserInput,
		SessionMode:         baseInput.SessionMode,
		Environment:         baseInput.Environment,
		UserAuthority:       baseInput.UserAuthority,
		ModelProfile:        baseInput.ModelProfile,
		AvailableConnectors: []string{"linear"},
	})
	if len(available.SelectedToolIDs) != 1 || available.SelectedToolIDs[0] != "navi.linear.create_issue" {
		t.Fatalf("expected connector-backed tool after dependency availability, got %+v", available.SelectedToolIDs)
	}
}

func TestToolBrokerResolveSuppressesOverlappingTools(t *testing.T) {
	reg := NewRegistry()
	availability := validTestTool("navi.calendar.read_availability")
	availability.DisplayName = "Calendar Availability"
	availability.Description = "Check calendar availability windows."
	availability.Aliases = []string{"calendar availability"}
	availability.CapabilityTags = []string{"calendar", "availability", "schedule"}
	availability.Governance.Domain = "calendar"
	availability.EnvironmentVisibility = []string{"production", "development"}
	if err := reg.Register(availability); err != nil {
		t.Fatalf("register availability tool: %v", err)
	}

	events := validTestTool("navi.calendar.list_events")
	events.DisplayName = "Calendar Events"
	events.Description = "List calendar events and schedule windows."
	events.Aliases = []string{"calendar events"}
	events.CapabilityTags = []string{"calendar", "events", "schedule"}
	events.Governance.Domain = "calendar"
	events.EnvironmentVisibility = []string{"production", "development"}
	if err := reg.Register(events); err != nil {
		t.Fatalf("register events tool: %v", err)
	}

	result := NewToolBrokerFromRegistry(reg).Resolve(BrokerInput{
		UserInput:     "check my calendar schedule and availability",
		SessionMode:   DiscoverySessionModeAssistant,
		Environment:   "production",
		UserAuthority: ToolAuthorityOwner,
		ModelProfile: BrokerModelProfile{
			Name:             "frontier_strong",
			SupportsTools:    true,
			ToolCallReliable: true,
		},
	})

	if len(result.SelectedToolIDs) != 1 {
		t.Fatalf("expected one selected calendar tool after overlap suppression, got %+v", result.SelectedToolIDs)
	}
	if !hasSuppressedTool(result.SuppressedTools, "navi.calendar.list_events", BrokerSuppressionTooManySimilarTools) &&
		!hasSuppressedTool(result.SuppressedTools, "navi.calendar.read_availability", BrokerSuppressionTooManySimilarTools) {
		t.Fatalf("expected overlapping calendar suppression, got %+v", result.SuppressedTools)
	}
}

func TestToolBrokerResolveWorkflowStateBoostsRelevantCandidate(t *testing.T) {
	reg := NewRegistry()
	calendar := validTestTool("navi.calendar.schedule_invite")
	calendar.DisplayName = "Schedule Invite"
	calendar.Description = "Schedule a meeting invite on the calendar."
	calendar.Aliases = []string{"schedule invite"}
	calendar.CapabilityTags = []string{"calendar", "invite", "schedule"}
	calendar.Governance.Domain = "calendar"
	calendar.EnvironmentVisibility = []string{"production", "development"}
	if err := reg.Register(calendar); err != nil {
		t.Fatalf("register calendar tool: %v", err)
	}

	message := validTestTool("navi.messaging.send_invite")
	message.DisplayName = "Send Invite"
	message.Description = "Send an invite message."
	message.Aliases = []string{"send invite"}
	message.CapabilityTags = []string{"message", "invite", "send"}
	message.Governance.Domain = "messaging"
	message.Category = ToolCategoryWorkflowAction
	message.EnvironmentVisibility = []string{"production", "development"}
	if err := reg.Register(message); err != nil {
		t.Fatalf("register message tool: %v", err)
	}

	result := NewToolBrokerFromRegistry(reg).Resolve(BrokerInput{
		UserInput:     "continue the invite step",
		Intent:        BrokerIntentAssistantTask,
		WorkflowState: "calendar scheduling phase",
		SessionMode:   DiscoverySessionModeAssistant,
		Environment:   "production",
		UserAuthority: ToolAuthorityOwner,
		ModelProfile: BrokerModelProfile{
			Name:             "frontier_strong",
			SupportsTools:    true,
			ToolCallReliable: true,
		},
	})

	if len(result.SelectedTools) == 0 || result.SelectedTools[0].ToolID != "navi.calendar.schedule_invite" {
		t.Fatalf("expected workflow-relevant calendar tool first, got %+v", result.SelectedTools)
	}
	if !containsString(result.SelectedTools[0].Rationale, "workflow_state_relevant") {
		t.Fatalf("expected workflow rationale on selected tool, got %+v", result.SelectedTools[0])
	}
}

func hasSuppressedTool(items []BrokerSuppressedTool, toolID string, reason BrokerSuppressionReason) bool {
	for _, item := range items {
		if item.ToolID == toolID && item.Reason == reason {
			return true
		}
	}
	return false
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func coderTool(id string, display string, description string, tags ...string) *Tool {
	toolEntry := validTestTool(id)
	toolEntry.DisplayName = display
	toolEntry.Description = description
	toolEntry.CapabilityTags = append([]string(nil), tags...)
	toolEntry.Governance.Domain = tags[0]
	toolEntry.Category = ToolCategoryReadOnly
	toolEntry.Metadata.ExposureClass = ToolExposureDevelopment
	toolEntry.EnvironmentVisibility = []string{"development", "test"}
	toolEntry.RequiredModes = []schema.DirectiveMode{schema.DirectiveModeAct}
	return toolEntry
}

func withModel(input BrokerInput, name string) BrokerInput {
	input.ModelProfile = BrokerModelProfile{
		Name:             name,
		SupportsTools:    true,
		ToolCallReliable: true,
	}
	return input
}
