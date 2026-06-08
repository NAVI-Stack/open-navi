import type {
  CapabilityGraph,
  CapabilityNode,
  ConnectorNode,
  DocNode,
  PluginNode,
  SkillNode,
  ToolInterfaceNode,
  UISurfaceNode,
} from '@/api/capabilities';

export interface PluginResources {
  plugin: PluginNode;
  skills: SkillNode[];
  toolInterfaces: ToolInterfaceNode[];
  connectors: ConnectorNode[];
  uiSurfaces: UISurfaceNode[];
  docs: DocNode[];
}

const nodeId = (n: { id?: string }) => (n.id ?? '').trim();

// belongsToPlugin matches a resource to a plugin either by its declared
// parentPluginId or by a graph edge from the plugin node to the resource.
function belongsToPlugin(
  node: CapabilityNode & { parentPluginId?: string },
  pluginId: string,
  pluginNodeId: string,
  edgeTargets: Set<string>,
): boolean {
  if (node.parentPluginId && node.parentPluginId === pluginId) return true;
  if (edgeTargets.has(nodeId(node))) return true;
  // Defensive: some builders only set the edge from the plugin node id.
  return nodeId(node).length > 0 && edgeTargets.has(pluginNodeId + '->' + nodeId(node));
}

// findPluginNode locates a plugin node by raw plugin id (pluginId field or node id suffix).
export function findPluginNode(graph: CapabilityGraph, pluginId: string): PluginNode | null {
  const want = pluginId.trim();
  for (const p of graph.plugins) {
    if (p.pluginId === want) return p;
    if (nodeId(p) === 'plugin:' + want) return p;
    if (nodeId(p) === want) return p;
  }
  return null;
}

// collectPluginResources groups every resource that belongs to the given plugin,
// using both parentPluginId and graph edges so the view does not depend on a
// single linking convention.
export function collectPluginResources(graph: CapabilityGraph, plugin: PluginNode): PluginResources {
  const pluginId = plugin.pluginId || nodeId(plugin).replace(/^plugin:/, '');
  const pNodeId = nodeId(plugin);

  const edgeTargets = new Set<string>();
  for (const edge of graph.edges) {
    if (edge.from === pNodeId) edgeTargets.add(edge.to);
  }

  const pick = <T extends CapabilityNode & { parentPluginId?: string }>(list: T[]): T[] =>
    list.filter((n) => belongsToPlugin(n, pluginId, pNodeId, edgeTargets));

  return {
    plugin,
    skills: pick(graph.skills),
    toolInterfaces: pick(graph.toolInterfaces),
    connectors: pick(graph.connectors),
    uiSurfaces: pick(graph.uiSurfaces),
    docs: pick(graph.docs),
  };
}
