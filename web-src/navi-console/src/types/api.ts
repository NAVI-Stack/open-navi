import { z } from 'zod';
import { ProposalSchema as GovernedProposalSchema } from '@/types/generated/governed';

export const GatewayInfoSchema = z.object({
  reachable: z.boolean(),
  version: z.string().optional(),
  build: z.string().optional(),
});

export const GovernorSchema = z.object({
  tripped: z.boolean(),
  budget_used: z.number(),
  budget_max: z.number(),
  cost_used: z.number(),
  cost_ceiling: z.number(),
  duration_remaining: z.string().optional(),
});

export type Governor = z.infer<typeof GovernorSchema>;

export const ConnectorItemSchema = z.object({
  name: z.string(),
  state: z.string(),
  last_activity_at: z.string().nullable().optional(),
  last_error_at: z.string().nullable().optional(),
  send_count: z.number().optional(),
  error_count: z.number().optional(),
  queue_depth: z.number().optional(),
});

export const StatusResponseSchema = z.object({
  gateway: GatewayInfoSchema,
  setup: z.object({ complete: z.boolean() }),
  identity: z.object({
    agent_id: z.string().optional(),
    owner_id: z.string().optional(),
    timezone: z.string().optional(),
  }),
  governor: GovernorSchema,
  proposals: z.object({ pending_count: z.number() }),
  connectors: z.object({
    total: z.number(),
    running: z.number(),
    degraded: z.number(),
    stopped: z.number(),
    error: z.number(),
    items: z.array(ConnectorItemSchema),
  }),
  llm: z.object({
    configured: z.boolean(),
    status: z.string().optional(),
  }),
  degraded: z.boolean(),
  updated_at: z.string(),
});

export type StatusResponse = z.infer<typeof StatusResponseSchema>;

export const AgentStatusResponseSchema = z.object({
  state: z.string(),
  uptime_since: z.string().optional(),
  turns_processed: z.number().optional(),
  active_runtime_session_id: z.string().optional(),
  current_detail: z.string().optional(),
  updated_at: z.string(),
}).passthrough();

export type AgentStatusResponse = z.infer<typeof AgentStatusResponseSchema>;

export const LlmActiveResponseSchema = z.object({
  provider: z.string().optional(),
  model: z.string().optional(),
  status: z.string().optional(),
}).passthrough();

export type LlmActiveResponse = z.infer<typeof LlmActiveResponseSchema>;

export const PresenceSnapshotSchema = z.record(z.unknown());
export type PresenceSnapshot = z.infer<typeof PresenceSnapshotSchema>;

export const ErrorSummaryItemSchema = z.object({
  type: z.string().optional(),
  component: z.string().optional(),
  count: z.number(),
  last_seen: z.string().optional(),
}).passthrough();

export const ErrorSummaryResponseSchema = z.object({
  total: z.number().optional(),
  window: z.string().optional(),
  items: z.array(ErrorSummaryItemSchema).optional(),
}).passthrough();

export type ErrorSummaryResponse = z.infer<typeof ErrorSummaryResponseSchema>;

// Proposal's governed core is generated from Go schema.Proposal (see
// @/types/generated/governed — do not hand-edit governed fields; Language-Layer
// Contract §7). The Console layers UI-only leniency on top: an `id` alias some
// operator surfaces read, all-optional access for partial/streaming payloads, and
// passthrough so the raw-proposal JSON panel keeps unknown keys.
export const ProposalSchema = GovernedProposalSchema.extend({
  id: z.string().optional(),
}).partial().passthrough();

export const ProposalListSchema = z.array(ProposalSchema);
export type Proposal = z.infer<typeof ProposalSchema>;

// --- CIP P5: intake sync policy + sync log (GET/POST /api/intake/*) ---

export const SyncBudgetSchema = z.object({
  max_records_per_pass: z.number().default(0),
  max_bytes_per_pass: z.number().default(0),
  cost_ceiling_usd: z.number().default(0),
}).passthrough();

export const SyncModePolicySchema = z.object({
  cadence: z.string().default(''),
  budget: SyncBudgetSchema.default({ max_records_per_pass: 0, max_bytes_per_pass: 0, cost_ceiling_usd: 0 }),
  cursor: z.string().default(''),
  dedupe: z.string().default(''),
  freshness: z.string().default(''),
}).passthrough();

export const SyncPolicySchema = z.object({
  connector_id: z.string().default(''),
  privacy_class: z.string().default(''),
  visibility: z.string().default(''),
  delta: SyncModePolicySchema,
  backfill: SyncModePolicySchema,
}).passthrough();

export const IntakeSyncLogEntrySchema = z.object({
  id: z.string().optional(),
  connector_id: z.string().optional(),
  parent_id: z.string().optional(),
  job_mode: z.string().optional(),
  started_at: z.string().optional(),
  ended_at: z.string().optional(),
  records_admitted: z.number().optional(),
  records_deduped: z.number().optional(),
  records_distilled: z.number().optional(),
  records_synthesized: z.number().optional(),
  errors: z.number().optional(),
  terminal_status: z.string().optional(),
  note: z.string().optional(),
  created_at: z.string().optional(),
}).passthrough();

export const ConnectorSyncViewSchema = z.object({
  connector_id: z.string(),
  policy: SyncPolicySchema,
  last_pass: IntakeSyncLogEntrySchema.nullish(),
  next_pass: z.string().nullish(),
  recent_passes: z.array(IntakeSyncLogEntrySchema).default([]),
  consent_state: z.string().optional(),
}).passthrough();

export const ConnectorSyncViewListSchema = z.array(ConnectorSyncViewSchema);

export const RecentIntakeRecordSchema = z.object({
  record_id: z.string(),
  connector_id: z.string(),
  source_kind: z.string().optional(),
  source_id: z.string().optional(),
  author: z.string().optional(),
  link_back: z.string().optional(),
  trust: z.string().optional(),
  privacy_class: z.string().optional(),
  fetched_at: z.string().optional(),
  chunk_count: z.number().optional(),
}).passthrough();

export const RecentIntakeListSchema = z.array(RecentIntakeRecordSchema);

export const VaultSyncLogEntrySchema = z.object({
  id: z.string().optional(),
  file_path: z.string().optional(),
  started_at: z.string().optional(),
  ended_at: z.string().optional(),
  diff_summary: z.string().optional(),
  mutations_proposed: z.number().optional(),
  mutations_approved: z.number().optional(),
  proposals_raised: z.number().optional(),
  errors: z.number().optional(),
  terminal_status: z.string().optional(),
  created_at: z.string().optional(),
}).passthrough();

export const VaultSyncLogListSchema = z.array(VaultSyncLogEntrySchema);

export type SyncPolicy = z.infer<typeof SyncPolicySchema>;
export type SyncModePolicy = z.infer<typeof SyncModePolicySchema>;
export type IntakeSyncLogEntry = z.infer<typeof IntakeSyncLogEntrySchema>;
export type ConnectorSyncView = z.infer<typeof ConnectorSyncViewSchema>;
export type RecentIntakeRecord = z.infer<typeof RecentIntakeRecordSchema>;
export type VaultSyncLogEntry = z.infer<typeof VaultSyncLogEntrySchema>;

// --- Activity feed (GET /api/activity) ---

export const ActivityItemSchema = z.object({
  at: z.string(),
  type: z.string(),
  summary: z.string(),
  correlation_id: z.string().optional(),
  id: z.string().optional(),
  chat_id: z.string().optional(),
  runtime_session_id: z.string().optional(),
  proposal_id: z.string().optional(),
  run_id: z.string().optional(),
  directive_id: z.string().optional(),
  connector_id: z.string().optional(),
  event_seq: z.number().optional(),
}).passthrough();

export type ActivityItem = z.infer<typeof ActivityItemSchema>;

export const ActivityResponseSchema = z.object({
  items: z.array(ActivityItemSchema),
  next_cursor: z.union([z.string(), z.number()]).optional(),
}).passthrough();

export type ActivityResponse = z.infer<typeof ActivityResponseSchema>;

// --- Debug events (GET /api/debug/events) ---

export const DebugEventItemSchema = z.object({
  id: z.string().optional(),
  type: z.string().optional(),
  kind: z.string().optional(),
  correlation_id: z.string().optional(),
  causal_parent: z.string().optional(),
  source_agent: z.string().optional(),
  target_agent: z.string().optional(),
  timestamp: z.string().optional(),
  payload: z.unknown().optional(),
  schema_version: z.string().optional(),
  seq: z.number().optional(),
  run_id: z.string().optional(),
  visibility: z.string().optional(),
}).passthrough();

export type DebugEventItem = z.infer<typeof DebugEventItemSchema>;

export const DebugEventsResponseSchema = z.object({
  items: z.array(DebugEventItemSchema),
  next_cursor: z.union([z.string(), z.number()]).optional(),
}).passthrough();

export type DebugEventsResponse = z.infer<typeof DebugEventsResponseSchema>;

// --- Error records (GET /api/errors) ---

export const ErrorRecordSchema = z.object({
  id: z.string().optional(),
  type: z.string().optional(),
  component: z.string().optional(),
  message: z.string().optional(),
  severity: z.string().optional(),
  chat_id: z.string().optional(),
  runtime_session_id: z.string().optional(),
  timestamp: z.string().optional(),
  error_type: z.string().optional(),
}).passthrough();

export type ErrorRecord = z.infer<typeof ErrorRecordSchema>;

export const ErrorsResponseSchema = z.object({
  items: z.array(ErrorRecordSchema),
}).passthrough();

export type ErrorsResponse = z.infer<typeof ErrorsResponseSchema>;

// --- Runtime metrics (GET /api/debug/runtime-metrics) ---

export const RuntimeMetricsSchema = z.record(z.unknown());
export type RuntimeMetrics = z.infer<typeof RuntimeMetricsSchema>;

// --- Chats & Runs ---

export const ChatEntrySchema = z.object({
  id: z.string().optional(),
  chat_id: z.string().optional().default(''),
  title: z.string().optional(),
  status: z.string().optional(),
  createdAt: z.string().optional(),
  updatedAt: z.string().optional(),
  projectId: z.string().optional(),
  created_at: z.string().optional(),
  updated_at: z.string().optional(),
  source: z.string().optional(),
  source_channel: z.string().optional(),
  project_id: z.string().optional(),
}).passthrough();

export type ChatEntry = z.infer<typeof ChatEntrySchema>;

export const ChatMessageSchema = z.object({
  id: z.string().optional(),
  chatId: z.string().optional(),
  chat_id: z.string().optional(),
  role: z.string().optional(),
  content: z.string().optional(),
  createdAt: z.string().optional(),
  created_at: z.string().optional(),
  runtimeSessionId: z.string().optional(),
  runtime_session_id: z.string().optional(),
  runId: z.string().optional(),
  run_id: z.string().optional(),
  inboxItemId: z.string().optional(),
  inbox_item_id: z.string().optional(),
  sourceChannel: z.string().optional(),
  source_channel: z.string().optional(),
  sourceMessageRef: z.string().optional(),
  source_message_ref: z.string().optional(),
  messageKind: z.string().optional(),
  message_kind: z.string().optional(),
  metadata: z.record(z.any()).optional(),
}).passthrough();

export type ChatMessage = z.infer<typeof ChatMessageSchema>;

export const MessageVariantSchema = z.object({
  id: z.string(),
  index: z.number(),
  content: z.string(),
}).passthrough();

export const MessageVariantsSchema = z.object({
  variants: z.array(MessageVariantSchema),
  selectedIndex: z.number(),
}).passthrough();

export type MessageVariant = z.infer<typeof MessageVariantSchema>;
export type MessageVariants = z.infer<typeof MessageVariantsSchema>;

export const RuntimeSummarySchema = z.object({
  chat_id: z.string().optional(),
  runtime_session_id: z.string().optional(),
  status: z.string().optional(),
  active_run_id: z.string().optional(),
  current_run_id: z.string().optional(),
  recent_run_ids: z.array(z.string()).optional(),
  created_at: z.string().optional(),
  updated_at: z.string().optional(),
}).passthrough();

export type RuntimeSummary = z.infer<typeof RuntimeSummarySchema>;

export const ChatThreadSchema = z.object({
  chat: ChatEntrySchema,
  messages: z.array(ChatMessageSchema).optional(),
}).passthrough();

export type ChatThread = z.infer<typeof ChatThreadSchema>;
export type ChatThreadView = ChatEntry & { messages: ChatMessage[] };

export const ChatSendResponseSchema = z.object({
  status: z.string().optional(),
  inbox_item_id: z.string().optional(),
  queue_action: z.string().optional(),
  inbox_status: z.string().optional(),
  classified_reason: z.string().optional(),
  merged_into_id: z.string().optional(),
  run_id: z.string().optional(),
  blocked_on_proposal_id: z.string().optional(),
  pause_reason: z.string().optional(),
  run_status: z.string().optional(),
}).passthrough();

export type ChatSendResponse = z.infer<typeof ChatSendResponseSchema>;

export const RunItemSchema = z.object({
  id: z.string().optional(),
  run_id: z.string().optional(),
  chat_id: z.string().optional(),
  runtime_session_id: z.string().optional(),
  status: z.string().optional(),
  created_at: z.string().optional(),
  updated_at: z.string().optional(),
}).passthrough();

export type RunItem = z.infer<typeof RunItemSchema>;

export const RunsResponseSchema = z.object({
  items: z.array(RunItemSchema),
  next_cursor: z.union([z.string(), z.number()]).optional(),
}).passthrough();

export type RunsResponse = z.infer<typeof RunsResponseSchema>;

// --- Config / Identity / Experience / Presence / LLM ---

export const AuthMeSchema = z.unknown();
export type AuthMe = z.infer<typeof AuthMeSchema>;

export const IdentitySchema = z.unknown();
export type Identity = z.infer<typeof IdentitySchema>;

export const ExperienceSchema = z.unknown();
export type Experience = z.infer<typeof ExperienceSchema>;

export const ExperienceInspectSchema = z.unknown();
export type ExperienceInspect = z.infer<typeof ExperienceInspectSchema>;

export const ExperienceModuleRegistrySchema = z.unknown();
export type ExperienceModuleRegistry = z.infer<typeof ExperienceModuleRegistrySchema>;

export const NaviPresencePublicStatusSchema = z.enum([
  'active',
  'idle',
  'dreaming',
  'working',
  'busy',
  'offline',
  'needs_attention',
  'wants_attention',
]);

export const NaviPresenceAttentionSchema = z.object({
  level: z.string(),
  reason_code: z.string().nullable().optional(),
  proposal_id: z.string().nullable().optional(),
  blocking: z.boolean(),
}).passthrough();

export const NaviPresenceHealthSchema = z.object({
  state: z.string(),
  last_activity_at: z.string().optional(),
  stale_after_ms: z.number(),
}).passthrough();

export const NaviPresencePayloadSchema = z.object({
  public_status: NaviPresencePublicStatusSchema,
  internal_status: z.string(),
  status_text: z.string().optional(),
  subtext: z.string().optional(),
  active_runtime_session_id: z.string().optional(),
  current_detail: z.string().optional(),
  attention: NaviPresenceAttentionSchema,
  health: NaviPresenceHealthSchema,
}).passthrough();

export const NaviPresenceSchema = z.object({
  version: z.string(),
  source: z.string(),
  subject_type: z.string(),
  subject_id: z.string(),
  authority: z.string(),
  transport_observed_at: z.string(),
  state_updated_at: z.string(),
  state_revision: z.number(),
  visibility: z.string(),
  payload: NaviPresencePayloadSchema,
}).passthrough();
export type NaviPresence = z.infer<typeof NaviPresenceSchema>;

export const RoutingVisibilitySchema = z.enum(['silent', 'thinking', 'debug', 'user']);
export type RoutingVisibility = z.infer<typeof RoutingVisibilitySchema>;

export const LlmPreferencesSchema = z.object({
  default_agentic: z.string().optional(),
  default_coding: z.string().optional(),
  default_chat: z.string().optional(),
  default_reasoning: z.string().optional(),
  default_lightweight: z.string().optional(),
  prefer_local: z.boolean().optional(),
  cost_sensitive: z.boolean().optional(),
  routing_visibility: RoutingVisibilitySchema.optional(),
  favorite_model_ids: z.array(z.string()).optional(),
  auto_routing_enabled: z.boolean().optional(),
}).passthrough();
export type LlmPreferences = z.infer<typeof LlmPreferencesSchema>;

export const LlmProviderPolicySchema = z.object({
  enabled: z.boolean(),
  local_first: z.boolean().optional(),
  allow_pull: z.boolean().optional(),
  allow_delete: z.boolean().optional(),
  auto_warm: z.boolean().optional(),
  allow_paid_remote: z.boolean().optional(),
  allow_fallback_from_local: z.boolean().optional(),
}).passthrough();

export const LlmProviderDescriptorSchema = z.object({
  key: z.string(),
  display_name: z.string(),
  kind: z.enum(['local', 'cloud', 'proxy']),
  enabled: z.boolean(),
  configured: z.boolean().optional(),
  healthy: z.boolean().optional(),
  capabilities: z.array(z.string()).optional(),
  model_count: z.number().optional(),
  policies: LlmProviderPolicySchema.optional(),
}).passthrough();
export type LlmProviderDescriptor = z.infer<typeof LlmProviderDescriptorSchema>;

export const LlmProvidersSchema = z.object({
  providers: z.array(LlmProviderDescriptorSchema),
}).passthrough();
export type LlmProviders = z.infer<typeof LlmProvidersSchema>;

export const LlmModelProfileSchema = z.object({
  provider_key: z.string(),
  model_id: z.string(),
  display_name: z.string(),
  supports_tools: z.boolean(),
  supports_stream: z.boolean(),
  agentic_score: z.number(),
  coding_score: z.number(),
  chat_score: z.number(),
  reasoning_score: z.number(),
  speed_score: z.number(),
  cost_score: z.number(),
  max_context_tokens: z.number(),
  tool_call_reliable: z.boolean(),
  tags: z.array(z.string()).optional(),
}).passthrough();
export type LlmModelProfile = z.infer<typeof LlmModelProfileSchema>;

export const LlmProfilesSchema = z.object({
  profiles: z.array(LlmModelProfileSchema),
}).passthrough();
export type LlmProfiles = z.infer<typeof LlmProfilesSchema>;

export const LlmModelDescriptorSchema = z.object({
  name: z.string(),
  size: z.number().optional(),
  family: z.string().optional(),
  parameter_size: z.string().optional(),
  quantization_level: z.string().optional(),
  digest: z.string().optional(),
}).passthrough();
export type LlmModelDescriptor = z.infer<typeof LlmModelDescriptorSchema>;

export const LlmProviderModelsSchema = z.object({
  models: z.array(LlmModelDescriptorSchema),
}).passthrough();
export type LlmProviderModels = z.infer<typeof LlmProviderModelsSchema>;

export const LlmProviderHealthSchema = z.object({
  provider: z.string(),
  healthy: z.boolean(),
  latency_ms: z.number().optional(),
  message: z.string().optional(),
  checked_at: z.string().optional(),
  model_count: z.number().optional(),
}).passthrough();
export type LlmProviderHealth = z.infer<typeof LlmProviderHealthSchema>;

// --- LLM-KB Profiles (GET /api/debug/llmkb/profiles) ---

export const LlmKbProfileSchema = z.object({
  id: z.string(),
  name: z.string().optional(),
  provider: z.string().optional(),
  model: z.string().optional(),
  status: z.string().optional(),
}).passthrough();

export const LlmKbProfilesResponseSchema = z.object({
  items: z.array(LlmKbProfileSchema),
}).passthrough();

export type LlmKbProfile = z.infer<typeof LlmKbProfileSchema>;
export type LlmKbProfilesResponse = z.infer<typeof LlmKbProfilesResponseSchema>;

// --- Operator Overview (GET /api/operator/overview) ---

export const OperatorOverviewSchema = z.object({
  status: StatusResponseSchema,
  agent: AgentStatusResponseSchema,
  proposals: z.object({
    pending_count: z.number(),
    items: z.array(ProposalSchema),
  }),
  runs: z.object({
    items: z.array(RunItemSchema),
  }),
  activity: z.object({
    items: z.array(ActivityItemSchema),
    next_cursor: z.union([z.string(), z.number()]).optional(),
  }),
  skills: z.unknown().optional(),
  updated_at: z.string(),
}).passthrough();

export type OperatorOverview = z.infer<typeof OperatorOverviewSchema>;

// --- Projects and Workspaces ---

export const ProjectSchema = z.object({
  project_id: z.string(),
  title: z.string(),
  slug: z.string().optional(),
  description: z.string().optional(),
  project_kind: z.string().optional(),
  status: z.string().optional(),
  health: z.string().optional(),
  workspace_id: z.string().optional(),
  icon: z.string().optional(),
  color: z.string().optional(),
  memory_scope: z.string().optional(),
  created_at: z.string().optional(),
  updated_at: z.string().optional(),
  created_by: z.string().optional(),
  attributes: z.record(z.unknown()).optional(),
}).passthrough();

export type Project = z.infer<typeof ProjectSchema>;

export const ProjectListResponseSchema = z.object({
  items: z.array(ProjectSchema),
}).passthrough();

export type ProjectListResponse = z.infer<typeof ProjectListResponseSchema>;

export const ProjectReadinessSchema = z.object({
  project_id: z.string(),
  project_kind: z.string().optional(),
  status: z.string().optional(),
  ready_for_chat: z.boolean().optional(),
  ready_for_planning: z.boolean().optional(),
  ready_for_coding: z.boolean().optional(),
  workspace_binding_required: z.boolean().optional(),
  workspace_id: z.string().optional(),
  degraded_reasons: z.array(z.string()).optional(),
}).passthrough();

export type ProjectReadiness = z.infer<typeof ProjectReadinessSchema>;

export const ProjectTaskSchema = z.object({
  task_id: z.string().optional(),
  id: z.string().optional(),
  title: z.string().optional(),
  description: z.string().optional(),
  status: z.string().optional(),
  risk: z.string().optional(),
  assigned_to: z.string().optional(),
  directive_id: z.string().optional(),
  project_id: z.string().optional(),
  workspace_id: z.string().optional(),
  task_class: z.string().optional(),
  raw_input: z.string().optional(),
  acceptance_target: z.string().optional(),
  lifecycle_phase: z.string().optional(),
  block_reason: z.string().optional(),
  created_at: z.string().optional(),
  updated_at: z.string().optional(),
}).passthrough();

export type ProjectTask = z.infer<typeof ProjectTaskSchema>;

export const ProjectTasksResponseSchema = z.object({
  project_id: z.string(),
  items: z.array(ProjectTaskSchema),
}).passthrough();

export type ProjectTasksResponse = z.infer<typeof ProjectTasksResponseSchema>;

export const ProjectChatsResponseSchema = z.object({
  project_id: z.string(),
  items: z.array(ChatEntrySchema),
}).passthrough();

export type ProjectChatsResponse = z.infer<typeof ProjectChatsResponseSchema>;

export const AllowedActionsSchema = z.object({
  read: z.boolean(),
  write: z.boolean(),
  create: z.boolean(),
  modify: z.boolean(),
  rename_move: z.boolean(),
  delete: z.boolean(),
  execute: z.boolean(),
}).passthrough();

export type AllowedActions = z.infer<typeof AllowedActionsSchema>;

export const BoundaryPolicySchema = z.object({
  out_of_scope_default: z.string(),
}).passthrough();

export type BoundaryPolicy = z.infer<typeof BoundaryPolicySchema>;

export const WhitelistRuleSchema = z.object({
  rule_id: z.string(),
  scope: z.string(),
  action_types: z.array(z.string()).optional(),
  status: z.string().optional(),
  created_at: z.string().optional(),
  revoked_at: z.string().nullable().optional(),
  expires_at: z.string().nullable().optional(),
  created_by: z.string().optional(),
}).passthrough();

export type WhitelistRule = z.infer<typeof WhitelistRuleSchema>;

export const WorkspaceSchema = z.object({
  workspace_id: z.string(),
  name: z.string(),
  description: z.string().optional(),
  workspace_kind: z.string().optional(),
  status: z.string().optional(),
  local_roots: z.array(z.string()).optional(),
  repo_roots: z.array(z.string()).optional(),
  protected_paths: z.array(z.string()).optional(),
  allowed_actions: AllowedActionsSchema.optional(),
  boundary_policy: BoundaryPolicySchema.optional(),
  audit_enabled: z.boolean().optional(),
  created_at: z.string().optional(),
  updated_at: z.string().optional(),
  created_by: z.string().optional(),
  tags: z.array(z.string()).optional(),
  related_project_id: z.string().optional(),
  notes: z.string().optional(),
  whitelist_rules: z.array(WhitelistRuleSchema).optional(),
  metadata: z.unknown().optional(),
}).passthrough();

export type Workspace = z.infer<typeof WorkspaceSchema>;

export const WorkspaceListResponseSchema = z.object({
  items: z.array(WorkspaceSchema),
  mode: z.string().optional(),
  active_workspace_id: z.string().optional(),
}).passthrough();

export type WorkspaceListResponse = z.infer<typeof WorkspaceListResponseSchema>;

export const WorkspaceModeResponseSchema = z.object({
  mode: z.string().optional(),
  selected_workspace_id: z.string().optional(),
  active_workspace_id: z.string().optional(),
  active_workspace_selection: z.string().optional(),
  project_context_related_id: z.string().optional(),
}).passthrough();

export type WorkspaceModeResponse = z.infer<typeof WorkspaceModeResponseSchema>;

export const ActiveWorkspaceResponseSchema = z.object({
  selected_workspace_id: z.string().optional(),
  active_workspace_id: z.string().optional(),
  active_workspace_selection: z.string().optional(),
  workspace: WorkspaceSchema.optional(),
}).passthrough();

export type ActiveWorkspaceResponse = z.infer<typeof ActiveWorkspaceResponseSchema>;

export const WhitelistRulesResponseSchema = z.object({
  items: z.array(WhitelistRuleSchema),
}).passthrough();

export type WhitelistRulesResponse = z.infer<typeof WhitelistRulesResponseSchema>;

export const WorkspacePathEntrySchema = z.object({
  name: z.string(),
  path: z.string(),
}).passthrough();

export type WorkspacePathEntry = z.infer<typeof WorkspacePathEntrySchema>;

export const WorkspacePathRootsResponseSchema = z.object({
  items: z.array(WorkspacePathEntrySchema),
  runtime_os: z.string().optional(),
  in_container: z.boolean().optional(),
  data_dir: z.string().optional(),
  workspace_dir: z.string().optional(),
}).passthrough();

export type WorkspacePathRootsResponse = z.infer<typeof WorkspacePathRootsResponseSchema>;

export const WorkspacePathChildrenResponseSchema = z.object({
  path: z.string(),
  parent: z.string().optional(),
  items: z.array(WorkspacePathEntrySchema),
}).passthrough();

export type WorkspacePathChildrenResponse = z.infer<typeof WorkspacePathChildrenResponseSchema>;

// --- Artifacts ---

export const ArtifactVersionSchema = z.object({
  id: z.string(),
  artifact_id: z.string(),
  version_number: z.number(),
  base_version_id: z.string().nullable().optional(),
  branch_id: z.string(),
  author_actor_type: z.string(),
  author_actor_id: z.string().optional(),
  commit_state: z.string(),
  validation_state: z.string().optional(),
  patch_strategy: z.string().optional(),
  created_at: z.string(),
  attributes: z.record(z.unknown()).optional(),
}).passthrough();

export type ArtifactVersion = z.infer<typeof ArtifactVersionSchema>;

export const ArtifactExportSchema = z.object({
  export_id: z.string(),
  artifact_id: z.string(),
  format: z.string(),
  target_kind: z.string(),
  status: z.string(),
  failure_class: z.string().optional(),
  created_at: z.string(),
  completed_at: z.string().optional(),
}).passthrough();

export type ArtifactExport = z.infer<typeof ArtifactExportSchema>;

export const ArtifactSchema = z.object({
  id: z.string(),
  workspace_id: z.string(),
  project_id: z.string().optional(),
  owner_id: z.string(),
  title: z.string(),
  canonical_title: z.string(),
  display_title: z.string().optional(),
  type: z.string(),
  kind: z.string().optional(),
  subtype: z.string(),
  lifecycle_state: z.string(),
  current_branch_id: z.string(),
  current_version_id: z.string(),
  head_version_number: z.number(),
  location: z.string().optional(),
  description: z.string().optional(),
  status: z.string().optional(),
  last_updated: z.string().optional(),
  shared_exported: z.boolean().optional(),
  sync_state: z.string().optional(),
  content_format: z.string().optional(),
  pending_execution_state: z.string().optional(),
  versions: z.array(ArtifactVersionSchema).optional(),
  attributes: z.record(z.unknown()).optional(),
  created_at: z.string().optional(),
}).passthrough();

export type Artifact = z.infer<typeof ArtifactSchema>;

export const ArtifactListResponseSchema = z.object({
  items: z.array(ArtifactSchema),
});

export type ArtifactListResponse = z.infer<typeof ArtifactListResponseSchema>;
