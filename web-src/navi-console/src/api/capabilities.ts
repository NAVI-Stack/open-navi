import { useQuery } from '@tanstack/react-query';
import { z } from 'zod';
import { naviFetch } from './client';

const defaultStatus = {
  lifecycle: 'unknown',
  validation: 'unknown',
  availability: 'unknown',
  runtime: 'unknown',
  auth: 'unknown',
  health: 'unknown',
  ui: 'none',
};

const defaultActions = {
  canEnable: false,
  canDisable: false,
  canValidate: false,
  canReload: false,
  canConfigure: false,
  canRestart: false,
  canInspect: false,
};

const fallbackString = (fallback = '') =>
  z.preprocess((value) => (typeof value === 'string' ? value : undefined), z.string().default(fallback));

const fallbackBool = z.preprocess((value) => (typeof value === 'boolean' ? value : undefined), z.boolean().default(false));

const StringArraySchema = z.preprocess(
  (value) => (Array.isArray(value) ? value.filter((item) => typeof item === 'string') : []),
  z.array(z.string()),
);

export const CapabilityStatusSchema = z.object({
  lifecycle: fallbackString(defaultStatus.lifecycle),
  validation: fallbackString(defaultStatus.validation),
  availability: fallbackString(defaultStatus.availability),
  runtime: fallbackString(defaultStatus.runtime),
  auth: fallbackString(defaultStatus.auth),
  health: fallbackString(defaultStatus.health),
  ui: fallbackString(defaultStatus.ui),
}).passthrough().catch(defaultStatus);

export const CapabilityActionsSchema = z.object({
  canEnable: fallbackBool,
  canDisable: fallbackBool,
  canValidate: fallbackBool,
  canReload: fallbackBool,
  canConfigure: fallbackBool,
  canRestart: fallbackBool,
  canInspect: fallbackBool,
}).passthrough().catch(defaultActions);

const CapabilityNodeBaseSchema = z.object({
  id: fallbackString('unknown'),
  displayName: fallbackString().optional(),
  kind: fallbackString().optional(),
  version: fallbackString().optional(),
  description: fallbackString().optional(),
  source: fallbackString().optional(),
  parentPluginId: fallbackString().optional(),
  status: CapabilityStatusSchema.default(defaultStatus),
  statusReasons: StringArraySchema,
  actions: CapabilityActionsSchema.default(defaultActions),
  risk: z.any().optional(),
  trustTier: fallbackString().optional(),
  tags: StringArraySchema,
  rawRef: z.record(z.any()).optional(),
}).passthrough();

export const PluginNodeSchema = CapabilityNodeBaseSchema.extend({
  pluginId: fallbackString(),
  categories: StringArraySchema,
  capabilities: StringArraySchema,
  components: z.any().optional(),
  configSchema: z.any().optional(),
}).passthrough();

export const SkillNodeSchema = CapabilityNodeBaseSchema.extend({
  skillId: fallbackString(),
  interfaces: StringArraySchema,
  requiredEnv: StringArraySchema.optional(),
  requiredBinaries: StringArraySchema.optional(),
}).passthrough();

export const ToolInterfaceNodeSchema = CapabilityNodeBaseSchema.extend({
  canonicalInterfaceId: fallbackString(),
  registeredToolName: fallbackString().optional(),
  skillId: fallbackString().optional(),
  interfaceName: fallbackString().optional(),
  toolId: fallbackString().optional(),
  transport: fallbackString().optional(),
  sourceType: fallbackString().optional(),
  inputSchema: z.any().optional(),
  outputSchema: z.any().optional(),
}).passthrough();

export const ConnectorNodeSchema = CapabilityNodeBaseSchema.extend({
  driverId: fallbackString().optional(),
  instanceId: fallbackString().optional(),
  category: fallbackString().optional(),
  capabilities: z.preprocess((value) => (Array.isArray(value) ? value : []), z.array(z.any())).optional(),
  setupSchema: z.any().optional(),
}).passthrough();

export const UISurfaceNodeSchema = CapabilityNodeBaseSchema.extend({
  surfaceId: fallbackString().optional(),
  title: fallbackString().optional(),
  ownerType: fallbackString().optional(),
  ownerId: fallbackString().optional(),
  targetSurfaces: StringArraySchema,
  renderMode: fallbackString().optional(),
  schema: z.any().optional(),
  actionBindings: z.preprocess((value) => (Array.isArray(value) ? value : []), z.array(z.any())),
}).passthrough();

export const DocNodeSchema = CapabilityNodeBaseSchema.extend({
  path: fallbackString().optional(),
  docType: fallbackString().optional(),
  ownerType: fallbackString().optional(),
  ownerId: fallbackString().optional(),
}).passthrough();

export const CapabilityEdgeSchema = z.object({
  from: fallbackString(),
  to: fallbackString(),
  kind: fallbackString(),
}).passthrough();

export const CapabilityGraphSchema = z.object({
  plugins: z.array(PluginNodeSchema).default([]),
  skills: z.array(SkillNodeSchema).default([]),
  toolInterfaces: z.array(ToolInterfaceNodeSchema).default([]),
  connectors: z.array(ConnectorNodeSchema).default([]),
  uiSurfaces: z.array(UISurfaceNodeSchema).default([]),
  docs: z.array(DocNodeSchema).default([]),
  edges: z.array(CapabilityEdgeSchema).default([]),
}).passthrough();

export type CapabilityStatus = z.output<typeof CapabilityStatusSchema>;
export type CapabilityActions = z.output<typeof CapabilityActionsSchema>;
export type PluginNode = z.output<typeof PluginNodeSchema>;
export type SkillNode = z.output<typeof SkillNodeSchema>;
export type ToolInterfaceNode = z.output<typeof ToolInterfaceNodeSchema>;
export type ConnectorNode = z.output<typeof ConnectorNodeSchema>;
export type UISurfaceNode = z.output<typeof UISurfaceNodeSchema>;
export type DocNode = z.output<typeof DocNodeSchema>;
export type CapabilityEdge = z.output<typeof CapabilityEdgeSchema>;
export type CapabilityGraph = z.output<typeof CapabilityGraphSchema>;

export type CapabilityNode =
  | PluginNode
  | SkillNode
  | ToolInterfaceNode
  | ConnectorNode
  | UISurfaceNode
  | DocNode;

export function useCapabilitiesGraph() {
  return useQuery({
    queryKey: ['capabilities-graph'],
    queryFn: async () => CapabilityGraphSchema.parse(await naviFetch<unknown>('/api/capabilities/graph')),
    retry: false,
  });
}
