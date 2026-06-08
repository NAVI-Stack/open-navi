import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { CapabilityGraph } from '@/api/capabilities';

// --- Mocks ---
const graphState: { data: CapabilityGraph | null; isLoading: boolean; isError: boolean } = {
  data: null,
  isLoading: false,
  isError: false,
};
vi.mock('@/api/capabilities', () => ({
  useCapabilitiesGraph: () => ({ ...graphState, error: null, refetch: vi.fn() }),
}));

const disablePlugin = vi.fn().mockResolvedValue({});
const enablePlugin = vi.fn().mockResolvedValue({});
const reloadPlugin = vi.fn().mockResolvedValue({});
const validatePlugin = vi.fn().mockResolvedValue({ valid: true, reasons: [] });
const setupConnector = vi.fn().mockResolvedValue({ status: 'ok' });
const invokeSkillInterface = vi.fn().mockResolvedValue({ status: 'success', payload: { contacts: [] } });
vi.mock('@/api/extensions', () => ({
  disablePlugin: (id: string) => disablePlugin(id),
  enablePlugin: (id: string) => enablePlugin(id),
  reloadPlugin: (id: string) => reloadPlugin(id),
  validatePlugin: (id: string) => validatePlugin(id),
  setupConnector: (...a: unknown[]) => setupConnector(...a),
  invokeSkillInterface: (...a: unknown[]) => invokeSkillInterface(...a),
}));

vi.mock('@/app/router', () => ({ useNavigate: () => vi.fn() }));

import { PluginDetailPage } from './PluginDetailPage';

function contactsGraph(): CapabilityGraph {
  return {
    plugins: [{
      id: 'plugin:navi.contacts', pluginId: 'navi.contacts', displayName: 'NAVI Contacts',
      description: 'Privacy-first contact management.', version: '1.0.0', kind: 'domain',
      trustTier: 'builtin', status: { lifecycle: 'enabled', validation: 'valid', availability: 'available' },
      statusReasons: [], actions: {}, categories: [], capabilities: [], tags: [],
      rawRef: { rootDir: 'plugins/navi-contacts' },
    }],
    skills: [{ id: 'skill:navi-contacts', skillId: 'navi-contacts', parentPluginId: 'navi.contacts', displayName: 'NAVI Contacts', interfaces: ['create_contact'], status: {}, statusReasons: [], actions: {}, tags: [] }],
    toolInterfaces: [{ id: 'tool_interface:navi-contacts.create_contact', canonicalInterfaceId: 'navi-contacts.create_contact', interfaceName: 'create_contact', parentPluginId: 'navi.contacts', displayName: 'create_contact', status: {}, statusReasons: [], actions: {}, tags: [] }],
    connectors: [],
    uiSurfaces: [{
      id: 'ui_surface:skill:navi-contacts:contacts.console', parentPluginId: 'navi.contacts',
      ownerId: 'navi-contacts', displayName: 'Contacts Console', title: 'NAVI Contacts',
      renderMode: 'headless', targetSurfaces: ['console'], actionBindings: [],
      schema: { type: 'entity_manager', entity: 'contact', list: { action: 'list_contacts', result_path: 'contacts', primary_field: 'name' } },
      status: {}, statusReasons: [], actions: {}, tags: [],
    }],
    docs: [],
    edges: [],
  } as unknown as CapabilityGraph;
}

function emptyGraph(): CapabilityGraph {
  return {
    plugins: [{ id: 'plugin:empty', pluginId: 'empty', displayName: 'Empty Plugin', kind: 'domain', status: { lifecycle: 'enabled' }, statusReasons: [], actions: {}, categories: [], capabilities: [], tags: [] }],
    skills: [], toolInterfaces: [], connectors: [], uiSurfaces: [], docs: [], edges: [],
  } as unknown as CapabilityGraph;
}

function telegramGraph(): CapabilityGraph {
  return {
    plugins: [{
      id: 'plugin:navi.connector.telegram',
      pluginId: 'navi.connector.telegram',
      displayName: 'Telegram Connector',
      description: 'Official Telegram bot connector.',
      version: '0.1.0',
      kind: 'integration',
      trustTier: 'builtin',
      status: { lifecycle: 'enabled', validation: 'valid', availability: 'available' },
      statusReasons: [],
      actions: {},
      categories: ['integration'],
      capabilities: ['messaging.send', 'messaging.receive'],
      tags: [],
      rawRef: { rootDir: 'plugins/telegram' },
    }],
    skills: [],
    toolInterfaces: [],
    connectors: [{
      id: 'connector:telegram',
      parentPluginId: 'navi.connector.telegram',
      displayName: 'Telegram',
      driverId: 'telegram',
      kind: 'connector_driver',
      category: 'messaging',
      status: { lifecycle: 'enabled', auth: 'unconfigured', runtime: 'stopped' },
      statusReasons: [],
      actions: { canConfigure: true },
      tags: [],
      setupSchema: {
        type: 'telegram',
        display_name: 'Telegram',
        required_params: [
          { key: 'bot_token', label: 'Bot Token', description: 'From @BotFather', secret: true },
        ],
        optional_params: [
          { key: 'owner_chat_id', label: 'Your Chat ID', description: 'From @userinfobot', secret: false },
        ],
        setup_hint: 'Get bot token from @BotFather and your numeric Chat ID from @userinfobot on Telegram.',
      },
    }],
    uiSurfaces: [],
    docs: [],
    edges: [],
  } as unknown as CapabilityGraph;
}

function renderPage(pluginId: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <PluginDetailPage pluginId={pluginId} />
    </QueryClientProvider>,
  );
}

describe('PluginDetailPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    graphState.isLoading = false;
    graphState.isError = false;
    graphState.data = contactsGraph();
  });

  it('renders the plugin header and grouped sections', async () => {
    renderPage('navi.contacts');
    expect(await screen.findByRole('heading', { name: 'NAVI Contacts' })).toBeInTheDocument();
    expect(screen.getByText('Skills')).toBeInTheDocument();
    expect(screen.getByText('Tool Calls')).toBeInTheDocument();
    expect(screen.getByText('Custom UI')).toBeInTheDocument();
    expect(screen.getByText('Validation & Status')).toBeInTheDocument();
  });

  it('shows a clean empty state for a plugin with no resources', () => {
    graphState.data = emptyGraph();
    renderPage('empty');
    expect(screen.getByText('No inspectable resources')).toBeInTheDocument();
    // The always-on status section is still present.
    expect(screen.getByText('Validation & Status')).toBeInTheDocument();
  });

  it('shows a loading state', () => {
    graphState.isLoading = true;
    graphState.data = null;
    renderPage('navi.contacts');
    expect(screen.getByText(/Loading plugin/i)).toBeInTheDocument();
  });

  it('shows a not-found state for an unknown plugin id', () => {
    renderPage('nope');
    expect(screen.getByText('Plugin not found')).toBeInTheDocument();
  });

  it('calls disablePlugin when the Disable action is clicked', async () => {
    renderPage('navi.contacts');
    await screen.findByRole('heading', { name: 'NAVI Contacts' });
    fireEvent.click(screen.getByRole('button', { name: /disable/i }));
    await waitFor(() => expect(disablePlugin).toHaveBeenCalledWith('navi.contacts'));
  });

  it('drives the Contact Manager via the skill-invoke endpoint', async () => {
    renderPage('navi.contacts');
    await waitFor(() => expect(invokeSkillInterface).toHaveBeenCalledWith('navi-contacts', 'list_contacts', {}));
  });

  it('lets users configure the Telegram bot token from plugin details', async () => {
    graphState.data = telegramGraph();
    renderPage('navi.connector.telegram');

    fireEvent.change(await screen.findByLabelText('Bot Token'), { target: { value: '123:abc' } });
    fireEvent.click(screen.getByRole('button', { name: /configure telegram/i }));

    await waitFor(() =>
      expect(setupConnector).toHaveBeenCalledWith('telegram', {
        bot_token: '123:abc',
      }),
    );
  });
});
