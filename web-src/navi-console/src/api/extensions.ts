import { useQuery } from '@tanstack/react-query';
import { z } from 'zod';
import { naviFetch } from './client';

// --- Schemas ---

// Skills
export const SkillSchema = z.object({
  id: z.string(),
  name: z.string().optional(),
  version: z.string().optional(),
  source: z.string().optional(),
  path: z.string().optional(),
  plugin_owner: z.string().optional(),
  enabled: z.boolean().optional(),
  governance: z.any().optional(),
  required_env: z.array(z.string()).optional(),
  required_binaries: z.array(z.string()).optional(),
  validation_status: z.string().optional(),
  last_error: z.any().optional(),
}).passthrough();

export type Skill = z.infer<typeof SkillSchema>;

export const SkillsListSchema = z.object({
  items: z.array(SkillSchema).default([]),
}).passthrough();

// Tools
export const ToolSchema = z.object({
  name: z.string(),
  description: z.string().optional(),
  category: z.string().optional(),
  input_schema: z.any().optional(),
  output_schema: z.any().optional(),
}).passthrough();

export type Tool = z.infer<typeof ToolSchema>;

export const ToolsListSchema = z.object({
  items: z.array(ToolSchema).default([]),
}).passthrough();

// Plugins
export const PluginManifestSchema = z.object({
  id: z.string(),
  name: z.string().optional(),
  kind: z.string().optional(),
  version: z.string().optional(),
  enabled: z.boolean().optional(),
  active: z.boolean().optional(),
  status: z.string().optional(),
  date_added: z.string().optional(),
  metadata: z.any().optional(),
  components: z.any().optional(),
}).passthrough();

export type PluginManifest = z.infer<typeof PluginManifestSchema>;

export const PluginsListSchema = z.object({
  plugins: z.array(PluginManifestSchema).default([]),
}).passthrough();

// Connectors
export const ConnectorInfoSchema = z.object({
  name: z.string(),
  state: z.string().optional(),
  status: z.string().optional(),
  connected_at: z.string().optional(),
  category: z.string().optional(),
}).passthrough();

export type ConnectorInfo = z.infer<typeof ConnectorInfoSchema>;

export const ConnectorInstanceV2Schema = z.object({
  instance_id: z.string(),
  driver_id: z.string(),
  status: z.string().optional(),
  health_state: z.string().optional(),
  auth_state: z.string().optional(),
  labels: z.record(z.string()).optional(),
  capabilities: z.array(z.any()).optional(),
}).passthrough();

export type ConnectorInstanceV2 = z.infer<typeof ConnectorInstanceV2Schema>;

export const ConnectorHealthSchema = z.object({
  name: z.string(),
  state: z.string().optional(),
  status: z.string().optional(),
  last_send_at: z.string().optional(),
  last_error_at: z.string().optional(),
  send_count: z.number().optional(),
  error_count: z.number().optional(),
  consecutive_errors: z.number().optional(),
  queue_depth: z.number().optional(),
}).passthrough();

export type ConnectorHealth = z.infer<typeof ConnectorHealthSchema>;

export const ConnectorDiagnosticSchema = z.object({
  name: z.string(),
  component: z.string().optional(),
  level: z.string().optional(),
  message: z.string().optional(),
  timestamp: z.string().optional(),
  data: z.any().optional(),
}).passthrough();

export type ConnectorDiagnostic = z.infer<typeof ConnectorDiagnosticSchema>;

// --- Hooks ---

// Skills
export function useSkills() {
  return useQuery({
    queryKey: ['skills'],
    queryFn: () => naviFetch('/api/skills', SkillsListSchema),
  });
}

export function useSkill(id: string | null) {
  return useQuery({
    queryKey: ['skill', id],
    queryFn: () => (id ? naviFetch(`/api/skills/${encodeURIComponent(id)}`, SkillSchema) : null),
    enabled: !!id,
  });
}

export async function reloadSkills() {
  return naviFetch('/api/skills/reload', z.any(), { method: 'POST' });
}

export async function validateSkill(id: string) {
  return naviFetch(`/api/skills/${encodeURIComponent(id)}/validate`, z.any(), { method: 'POST' });
}

// Tools
export function useTools() {
  return useQuery({
    queryKey: ['tools'],
    queryFn: () => naviFetch('/api/tools', ToolsListSchema),
  });
}

export function useTool(name: string | null) {
  return useQuery({
    queryKey: ['tool', name],
    queryFn: () => (name ? naviFetch(`/api/tools/${encodeURIComponent(name)}`, ToolSchema) : null),
    enabled: !!name,
  });
}

// Plugins
export function usePlugins() {
  return useQuery({
    queryKey: ['plugins'],
    queryFn: () => naviFetch('/api/plugins', PluginsListSchema),
  });
}

export async function enablePlugin(id: string) {
  return naviFetch(`/api/plugins/${encodeURIComponent(id)}/enable`, z.any(), { method: 'POST' });
}

export async function disablePlugin(id: string) {
  return naviFetch(`/api/plugins/${encodeURIComponent(id)}/disable`, z.any(), { method: 'POST' });
}

export const PluginValidationSchema = z.object({
  pluginId: z.string().optional(),
  valid: z.boolean().default(false),
  reasons: z.array(z.string()).default([]),
}).passthrough();

export type PluginValidation = z.infer<typeof PluginValidationSchema>;

export async function validatePlugin(id: string): Promise<PluginValidation> {
  return naviFetch(`/api/plugins/${encodeURIComponent(id)}/validate`, PluginValidationSchema, { method: 'POST' });
}

export async function reloadPlugin(id: string) {
  return naviFetch(`/api/plugins/${encodeURIComponent(id)}/reload`, z.any(), { method: 'POST' });
}

export async function setupConnector(type: string, params: Record<string, string>) {
  return naviFetch('/api/setup/connector', z.any(), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ type, params }),
  });
}

export const SkillInvokeResultSchema = z.object({
  status: z.string().optional(),
  payload: z.any().optional(),
  error: z.any().optional(),
}).passthrough();

export type SkillInvokeResult = z.infer<typeof SkillInvokeResultSchema>;

// invokeSkillInterface calls a governed skill interface and returns the parsed result.
// Powers schema-driven custom UI (e.g. the Contacts entity manager).
export async function invokeSkillInterface(
  skillId: string,
  iface: string,
  args: Record<string, unknown> = {},
): Promise<SkillInvokeResult> {
  return naviFetch(
    `/api/skills/${encodeURIComponent(skillId)}/interfaces/${encodeURIComponent(iface)}/invoke`,
    SkillInvokeResultSchema,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ arguments: args }),
    },
  );
}

// Connectors
export function useConnectors() {
  return useQuery({
    queryKey: ['connectors'],
    queryFn: () => naviFetch('/api/connectors', z.array(ConnectorInfoSchema)),
  });
}

export function useConnectorFactories() {
  return useQuery({
    queryKey: ['connector-factories'],
    queryFn: () => naviFetch('/api/connectors/factories', z.array(z.string())),
  });
}

export function useConnectorInstances() {
  return useQuery({
    queryKey: ['connector-instances'],
    queryFn: () => naviFetch('/v2/api/connectors/instances', z.array(ConnectorInstanceV2Schema)),
  });
}

export function useConnectorHealth() {
  return useQuery({
    queryKey: ['connector-health'],
    queryFn: () => naviFetch('/api/health/connectors', z.array(ConnectorHealthSchema)),
  });
}

export function useConnectorDiagnostics() {
  return useQuery({
    queryKey: ['connector-diagnostics'],
    queryFn: () => naviFetch('/api/diagnostics/connectors', z.array(ConnectorDiagnosticSchema)),
  });
}
