import { describe, expect, it } from 'vitest';
import type { CapabilityGraph } from '@/api/capabilities';
import { collectPluginResources, findPluginNode } from './resources';

// Minimal graph: one plugin owning a skill (by parentPluginId), a tool interface
// (by parentPluginId), a connector linked ONLY by edge, a UI surface and a doc;
// plus a foreign skill that belongs to a different plugin.
function makeGraph(): CapabilityGraph {
  return {
    plugins: [
      { id: 'plugin:navi.contacts', pluginId: 'navi.contacts', displayName: 'NAVI Contacts', kind: 'domain', status: {}, statusReasons: [], actions: {}, categories: [], capabilities: [], tags: [] },
      { id: 'plugin:other', pluginId: 'other', displayName: 'Other', kind: 'domain', status: {}, statusReasons: [], actions: {}, categories: [], capabilities: [], tags: [] },
    ],
    skills: [
      { id: 'skill:navi-contacts', skillId: 'navi-contacts', parentPluginId: 'navi.contacts', displayName: 'NAVI Contacts', interfaces: [], status: {}, statusReasons: [], actions: {}, tags: [] },
      { id: 'skill:foreign', skillId: 'foreign', parentPluginId: 'other', displayName: 'Foreign', interfaces: [], status: {}, statusReasons: [], actions: {}, tags: [] },
    ],
    toolInterfaces: [
      { id: 'tool_interface:navi-contacts.create', canonicalInterfaceId: 'navi-contacts.create', parentPluginId: 'navi.contacts', displayName: 'create', status: {}, statusReasons: [], actions: {}, tags: [] },
    ],
    connectors: [
      // No parentPluginId — should still attach via the edge below.
      { id: 'connector:c1', displayName: 'C1', status: {}, statusReasons: [], actions: {}, tags: [] },
    ],
    uiSurfaces: [
      { id: 'ui_surface:skill:navi-contacts:contacts.console', parentPluginId: 'navi.contacts', displayName: 'Contacts Console', targetSurfaces: [], actionBindings: [], status: {}, statusReasons: [], actions: {}, tags: [] },
    ],
    docs: [
      { id: 'doc:plugin:navi.contacts:README', parentPluginId: 'navi.contacts', displayName: 'README', status: {}, statusReasons: [], actions: {}, tags: [] },
    ],
    edges: [
      { from: 'plugin:navi.contacts', to: 'connector:c1', kind: 'plugin_uses_connector' },
    ],
  } as unknown as CapabilityGraph;
}

describe('findPluginNode', () => {
  it('finds a plugin by raw pluginId', () => {
    const node = findPluginNode(makeGraph(), 'navi.contacts');
    expect(node?.pluginId).toBe('navi.contacts');
  });

  it('returns null for unknown plugin', () => {
    expect(findPluginNode(makeGraph(), 'does.not.exist')).toBeNull();
  });
});

describe('collectPluginResources', () => {
  it('groups resources by parentPluginId and edges, excluding foreign ones', () => {
    const graph = makeGraph();
    const plugin = findPluginNode(graph, 'navi.contacts')!;
    const r = collectPluginResources(graph, plugin);

    expect(r.skills.map((s) => s.skillId)).toEqual(['navi-contacts']);
    expect(r.toolInterfaces).toHaveLength(1);
    expect(r.uiSurfaces).toHaveLength(1);
    expect(r.docs).toHaveLength(1);
    // Connector attached purely via the plugin→connector edge.
    expect(r.connectors.map((c) => c.id)).toEqual(['connector:c1']);
  });

  it('returns empty arrays for a plugin with no resources', () => {
    const graph = makeGraph();
    const plugin = findPluginNode(graph, 'other')!;
    const r = collectPluginResources(graph, plugin);
    expect(r.toolInterfaces).toHaveLength(0);
    expect(r.connectors).toHaveLength(0);
    expect(r.uiSurfaces).toHaveLength(0);
    // 'other' owns the foreign skill only.
    expect(r.skills.map((s) => s.skillId)).toEqual(['foreign']);
  });
});
