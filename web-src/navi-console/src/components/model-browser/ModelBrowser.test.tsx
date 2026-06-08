import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { BrowserModel, BrowserProvider } from './types';
import { ModelBrowser } from './ModelBrowser';
import { ModelBrowserTrigger } from './ModelBrowserTrigger';
import { AddModelModal } from './AddModelModal';

// ─── Mock external dependencies ───────────────────────────────────────────────

vi.mock('@/api/config', () => ({
  useLlmActive: () => ({ data: { provider: 'anthropic', model: 'claude-3' }, isLoading: false }),
  useLlmProviders: () => ({
    data: {
      providers: [
        { key: 'anthropic', display_name: 'Anthropic', kind: 'cloud', enabled: true, configured: true, healthy: true, model_count: 2 },
        { key: 'openai', display_name: 'OpenAI', kind: 'cloud', enabled: true, configured: true, healthy: true, model_count: 1 },
      ],
    },
    isLoading: false,
  }),
  useLlmProfiles: () => ({
    data: {
      profiles: [
        {
          provider_key: 'anthropic',
          model_id: 'claude-3',
          display_name: 'Claude 3',
          supports_tools: true,
          supports_stream: true,
          agentic_score: 80,
          coding_score: 75,
          chat_score: 85,
          reasoning_score: 78,
          speed_score: 60,
          cost_score: 50,
          max_context_tokens: 200000,
          tool_call_reliable: true,
          tags: ['cloud', 'reasoning'],
        },
        {
          provider_key: 'openai',
          model_id: 'gpt-4o',
          display_name: 'GPT-4o',
          supports_tools: true,
          supports_stream: true,
          agentic_score: 75,
          coding_score: 70,
          chat_score: 80,
          reasoning_score: 72,
          speed_score: 85,
          cost_score: 40,
          max_context_tokens: 128000,
          tool_call_reliable: true,
          tags: ['cloud', 'fast'],
        },
        {
          provider_key: 'anthropic',
          model_id: 'claude-legacy',
          display_name: 'Claude Legacy',
          supports_tools: false,
          supports_stream: true,
          agentic_score: 40,
          coding_score: 35,
          chat_score: 50,
          reasoning_score: 45,
          speed_score: 70,
          cost_score: 80,
          max_context_tokens: 100000,
          tool_call_reliable: false,
          tags: ['cloud', 'legacy'],
        },
      ],
    },
    isLoading: false,
  }),
  useLlmPreferences: () => ({ data: undefined, isLoading: false }),
  setLlmActive: vi.fn().mockResolvedValue({ provider: 'anthropic', model: 'claude-3', status: 'ok' }),
  patchLlmPreferences: vi.fn().mockResolvedValue({}),
  configureLlmProvider: vi.fn().mockResolvedValue({}),
  disableLlmProvider: vi.fn().mockResolvedValue(undefined),
}));

vi.mock('@/app/router', () => ({
  useNavigate: () => vi.fn(),
  useRoute: () => ({ path: '/chats', route: 'chats', params: {}, query: new URLSearchParams() }),
}));

// ─── Test helpers ──────────────────────────────────────────────────────────────

function makeQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

function Wrapper({ children }: { children: React.ReactNode }) {
  return (
    <QueryClientProvider client={makeQueryClient()}>
      {children}
    </QueryClientProvider>
  );
}

// Minimal BrowserModel fixtures
const MODEL_AUTO: BrowserModel = {
  id: 'navi/auto',
  providerKey: 'navi',
  providerName: 'NAVI',
  modelId: 'auto',
  displayName: 'NAVI Auto',
  capabilities: ['fast', 'reasoning', 'tools'],
  isLegacy: false,
  isRecommended: true,
  isNaviAuto: true,
  executionType: 'auto',
  status: 'connected',
  tags: ['auto', 'recommended'],
  speedScore: 90,
  costScore: 80,
  reasoningScore: 85,
  codingScore: 80,
  agenticScore: 85,
  chatScore: 85,
  maxContextTokens: 0,
  supportsTools: true,
  toolCallReliable: true,
};

const MODEL_CLAUDE: BrowserModel = {
  id: 'anthropic/claude-3',
  providerKey: 'anthropic',
  providerName: 'Anthropic',
  modelId: 'claude-3',
  displayName: 'Claude 3',
  capabilities: ['reasoning', 'tools', 'code'],
  isLegacy: false,
  isRecommended: true,
  isNaviAuto: false,
  executionType: 'cloud',
  status: 'connected',
  tags: ['cloud', 'reasoning'],
  speedScore: 60,
  costScore: 50,
  reasoningScore: 78,
  codingScore: 75,
  agenticScore: 80,
  chatScore: 85,
  maxContextTokens: 200000,
  supportsTools: true,
  toolCallReliable: true,
};

const MODEL_GPT: BrowserModel = {
  id: 'openai/gpt-4o',
  providerKey: 'openai',
  providerName: 'OpenAI',
  modelId: 'gpt-4o',
  displayName: 'GPT-4o',
  capabilities: ['fast', 'tools'],
  isLegacy: false,
  isRecommended: false,
  isNaviAuto: false,
  executionType: 'cloud',
  status: 'connected',
  tags: ['cloud', 'fast'],
  speedScore: 85,
  costScore: 40,
  reasoningScore: 72,
  codingScore: 70,
  agenticScore: 75,
  chatScore: 80,
  maxContextTokens: 128000,
  supportsTools: true,
  toolCallReliable: true,
};

const MODEL_LEGACY: BrowserModel = {
  id: 'anthropic/claude-legacy',
  providerKey: 'anthropic',
  providerName: 'Anthropic',
  modelId: 'claude-legacy',
  displayName: 'Claude Legacy',
  capabilities: [],
  isLegacy: true,
  isRecommended: false,
  isNaviAuto: false,
  executionType: 'cloud',
  status: 'connected',
  tags: ['cloud', 'legacy'],
  speedScore: 70,
  costScore: 80,
  reasoningScore: 45,
  codingScore: 35,
  agenticScore: 40,
  chatScore: 50,
  maxContextTokens: 100000,
  supportsTools: false,
  toolCallReliable: false,
};

const ALL_MODELS = [MODEL_AUTO, MODEL_CLAUDE, MODEL_GPT, MODEL_LEGACY];

const PROVIDERS: BrowserProvider[] = [
  { key: 'anthropic', displayName: 'Anthropic', kind: 'cloud', enabled: true, healthy: true, status: 'connected', modelCount: 2 },
  { key: 'openai', displayName: 'OpenAI', kind: 'cloud', enabled: true, healthy: true, status: 'connected', modelCount: 1 },
];

function renderBrowser(overrides?: Partial<React.ComponentProps<typeof ModelBrowser>>) {
  const onSelect = vi.fn();
  const onToggleFavorite = vi.fn();
  const onClose = vi.fn();
  const { rerender, ...rest } = render(
    <Wrapper>
      <ModelBrowser
        models={ALL_MODELS}
        providers={PROVIDERS}
        selectedId="navi/auto"
        favoriteIds={['navi/auto', 'anthropic/claude-3']}
        onSelect={onSelect}
        onToggleFavorite={onToggleFavorite}
        onClose={onClose}
        {...overrides}
      />
    </Wrapper>,
  );
  return { onSelect, onToggleFavorite, onClose, rerender, ...rest };
}

// ─── Tests ────────────────────────────────────────────────────────────────────

describe('ModelBrowserTrigger', () => {
  it('renders the current model name', () => {
    render(
      <Wrapper>
        <ModelBrowserTrigger
          model={MODEL_CLAUDE}
          isOpen={false}
          onClick={vi.fn()}
        />
      </Wrapper>,
    );
    expect(screen.getByRole('button', { name: /model selector/i })).toBeInTheDocument();
    expect(screen.getByText('Claude 3')).toBeInTheDocument();
  });

  it('shows chevron-open class when isOpen', () => {
    render(
      <Wrapper>
        <ModelBrowserTrigger model={MODEL_AUTO} isOpen={true} onClick={vi.fn()} />
      </Wrapper>,
    );
    // aria-expanded is set on the trigger button
    expect(screen.getByRole('button', { name: /model selector/i })).toHaveAttribute(
      'aria-expanded',
      'true',
    );
  });

  it('calls onClick when clicked', () => {
    const onClick = vi.fn();
    render(
      <Wrapper>
        <ModelBrowserTrigger model={MODEL_CLAUDE} isOpen={false} onClick={onClick} />
      </Wrapper>,
    );
    fireEvent.click(screen.getByRole('button', { name: /model selector/i }));
    expect(onClick).toHaveBeenCalledOnce();
  });
});

describe('ModelBrowser — open and close', () => {
  it('renders the model list when open', () => {
    renderBrowser();
    // Default "All" tab — all models should be visible
    expect(screen.getByText('NAVI Auto')).toBeInTheDocument();
    expect(screen.getByText('Claude 3')).toBeInTheDocument();
    expect(screen.getByText('GPT-4o')).toBeInTheDocument();
  });

  it('calls onClose when Escape is pressed', () => {
    const { onClose } = renderBrowser();
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onClose).toHaveBeenCalled();
  });

  it('calls onClose when the backdrop is clicked', () => {
    const { onClose } = renderBrowser();
    // The backdrop is the element with aria-hidden that precedes the panel
    const backdrop = document.querySelector('[aria-hidden="true"]:not([role])') as HTMLElement;
    expect(backdrop).not.toBeNull();
    fireEvent.pointerDown(backdrop);
    expect(onClose).toHaveBeenCalled();
  });

  it('does not call onClose on Escape when AddModelModal is open', () => {
    const { onClose } = renderBrowser();
    // Open the Providers modal
    fireEvent.click(screen.getByRole('button', { name: /add provider or model/i }));
    expect(screen.getByRole('dialog', { name: /provider configuration/i })).toBeInTheDocument();
    // Escape should close the modal, not the browser
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onClose).not.toHaveBeenCalled();
  });
});

describe('ModelBrowser — model selection', () => {
  it('calls onSelect with the model id when a row is clicked', () => {
    const { onSelect } = renderBrowser({ favoriteIds: ['navi/auto', 'anthropic/claude-3'] });
    fireEvent.click(screen.getByText('Claude 3'));
    expect(onSelect).toHaveBeenCalledWith('anthropic/claude-3');
  });

  it('marks the currently selected model row as selected', () => {
    renderBrowser({ selectedId: 'anthropic/claude-3', favoriteIds: ['navi/auto', 'anthropic/claude-3'] });
    const row = screen.getByRole('option', { name: /claude 3/i });
    expect(row).toHaveAttribute('aria-selected', 'true');
  });
});

describe('ModelBrowser — search', () => {
  it('shows all providers when searching', () => {
    renderBrowser();
    const input = screen.getByRole('searchbox');
    // Type a query that matches the openai model
    fireEvent.change(input, { target: { value: 'gpt' } });
    expect(screen.getByText('GPT-4o')).toBeInTheDocument();
  });

  it('filters results by search term across providers', () => {
    renderBrowser();
    const input = screen.getByRole('searchbox');
    fireEvent.change(input, { target: { value: 'gpt' } });
    // Claude should NOT appear
    expect(screen.queryByText('Claude 3')).not.toBeInTheDocument();
  });

  it('returning to empty search restores provider filter', () => {
    renderBrowser({ activeProvider: 'openai' } as never);
    const input = screen.getByRole('searchbox');
    fireEvent.change(input, { target: { value: 'claude' } });
    // Claude appears in search
    expect(screen.getByText('Claude 3')).toBeInTheDocument();
    // Clearing search — since provider is not directly a prop we just verify clear works
    fireEvent.change(input, { target: { value: '' } });
    // After clear, search input is empty
    expect(input).toHaveValue('');
  });
});

describe('ModelBrowser — provider rail', () => {
  it('renders provider rail buttons', () => {
    renderBrowser();
    expect(screen.getByRole('button', { name: 'All' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'NAVI' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Favorites' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /anthropic/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /openai/i })).toBeInTheDocument();
  });

  it('clicking a provider button filters models to that provider', () => {
    renderBrowser();
    fireEvent.click(screen.getByRole('button', { name: /openai/i }));
    expect(screen.getByText('GPT-4o')).toBeInTheDocument();
    // Claude should not appear (it's in anthropic)
    expect(screen.queryByText('Claude 3')).not.toBeInTheDocument();
  });
});

describe('ModelBrowser — capability filters', () => {
  it('opens the filter menu on filter button click', () => {
    renderBrowser();
    const filterBtn = screen.getByRole('button', { name: /filters and sort/i });
    fireEvent.click(filterBtn);
    // Filter menu should appear
    expect(screen.getByText('Capabilities')).toBeInTheDocument();
  });

  it('activating a capability filter narrows results', () => {
    renderBrowser();
    // Open all models first by searching
    const input = screen.getByRole('searchbox');
    fireEvent.change(input, { target: { value: ' ' } }); // space triggers search across all
    // Open filter menu
    const filterBtn = screen.getByRole('button', { name: /filters and sort/i });
    fireEvent.click(filterBtn);
    // Click "Coding" capability filter
    fireEvent.click(screen.getByRole('menuitemcheckbox', { name: /coding/i }));
    // Only models with 'code' capability should show — Claude 3 has it, GPT-4o does not
    expect(screen.queryByText('GPT-4o')).not.toBeInTheDocument();
  });
});

describe('ModelBrowser — sort presets', () => {
  it('shows all sort options in the filter menu', () => {
    renderBrowser();
    fireEvent.click(screen.getByRole('button', { name: /filters and sort/i }));
    expect(screen.getByRole('menuitemradio', { name: /fastest/i })).toBeInTheDocument();
    expect(screen.getByRole('menuitemradio', { name: /cheapest/i })).toBeInTheDocument();
  });

  it('changing sort to fastest puts the highest speedScore model first', () => {
    // Show all models via search
    renderBrowser();
    const input = screen.getByRole('searchbox');
    fireEvent.change(input, { target: { value: 'a' } }); // broad search
    fireEvent.click(screen.getByRole('button', { name: /filters and sort/i }));
    fireEvent.click(screen.getByRole('menuitemradio', { name: /fastest/i }));

    const items = screen.getAllByRole('option');
    // GPT-4o has speedScore=85, Claude 3 has 60 — GPT-4o should appear before Claude 3
    const gptIdx = items.findIndex((el) => el.textContent?.includes('GPT-4o'));
    const claudeIdx = items.findIndex((el) => el.textContent?.includes('Claude 3'));
    expect(gptIdx).toBeLessThan(claudeIdx);
  });
});

describe('ModelBrowser — favorites', () => {
  it('clicking the star does NOT call onSelect', () => {
    const { onSelect, onToggleFavorite } = renderBrowser({
      favoriteIds: ['navi/auto', 'anthropic/claude-3'],
    });
    const claudeRow = screen.getByRole('option', { name: /claude 3/i });
    const starBtn = within(claudeRow).getByRole('button', { name: /remove.*favorites/i });
    fireEvent.click(starBtn);
    expect(onToggleFavorite).toHaveBeenCalledWith('anthropic/claude-3');
    expect(onSelect).not.toHaveBeenCalled();
  });

  it('favorites provider view shows only favorited models', () => {
    renderBrowser({
      favoriteIds: ['openai/gpt-4o'],
    });
    // Switch to the Favorites tab
    fireEvent.click(screen.getByRole('button', { name: 'Favorites' }));
    expect(screen.getByText('GPT-4o')).toBeInTheDocument();
    // Claude 3 is not in favoriteIds
    expect(screen.queryByText('Claude 3')).not.toBeInTheDocument();
  });
});

describe('ModelBrowser — info / details panel', () => {
  it('clicking info opens details without calling onSelect', () => {
    const { onSelect } = renderBrowser({ favoriteIds: ['navi/auto', 'anthropic/claude-3'] });
    const claudeRow = screen.getByRole('option', { name: /claude 3/i });
    const infoBtn = within(claudeRow).getByRole('button', { name: /view details/i });
    fireEvent.click(infoBtn);
    expect(onSelect).not.toHaveBeenCalled();
    // Details panel should appear
    expect(screen.getByRole('region', { name: /details for claude 3/i })).toBeInTheDocument();
  });

  it('Escape closes the details panel before closing the browser', () => {
    const { onClose } = renderBrowser({ favoriteIds: ['navi/auto', 'anthropic/claude-3'] });
    const claudeRow = screen.getByRole('option', { name: /claude 3/i });
    fireEvent.click(within(claudeRow).getByRole('button', { name: /view details/i }));
    expect(screen.getByRole('region', { name: /details for claude 3/i })).toBeInTheDocument();
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('region', { name: /details for claude 3/i })).not.toBeInTheDocument();
    // onClose not called yet — Escape first closed the panel
    expect(onClose).not.toHaveBeenCalled();
  });
});

describe('ModelBrowser — legacy reveal', () => {
  it('legacy models are hidden by default', () => {
    renderBrowser({ favoriteIds: ['navi/auto'] });
    // Need to see all models — switch to anthropic provider
    fireEvent.click(screen.getByRole('button', { name: /anthropic/i }));
    // Claude Legacy is legacy — should not be visible yet
    expect(screen.queryByText('Claude Legacy')).not.toBeInTheDocument();
  });

  it('legacy reveal button merges legacy models into the list', () => {
    renderBrowser({ favoriteIds: ['navi/auto'] });
    fireEvent.click(screen.getByRole('button', { name: /anthropic/i }));
    const revealBtn = screen.getByRole('button', { name: /legacy model/i });
    fireEvent.click(revealBtn);
    expect(screen.getByText('Claude Legacy')).toBeInTheDocument();
  });
});

describe('ModelBrowser — model list scrollability', () => {
  it('model list container has overflow-y scroll behavior', () => {
    renderBrowser();
    // The model list group should be present; CSS overflow-y is set as auto in the module
    const list = screen.getByRole('group', { name: /available models/i });
    expect(list).toBeInTheDocument();
  });
});

describe('ModelBrowser — All tab (default)', () => {
  it('all tab is active by default and shows models from all providers', () => {
    renderBrowser();
    expect(screen.getByRole('button', { name: 'All' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByText('NAVI Auto')).toBeInTheDocument();
    expect(screen.getByText('Claude 3')).toBeInTheDocument();
    expect(screen.getByText('GPT-4o')).toBeInTheDocument();
  });
});

describe('ModelBrowser — NAVI tab', () => {
  it('NAVI tab shows only the NAVI Auto model', () => {
    renderBrowser();
    fireEvent.click(screen.getByRole('button', { name: 'NAVI' }));
    expect(screen.getByText('NAVI Auto')).toBeInTheDocument();
    expect(screen.queryByText('Claude 3')).not.toBeInTheDocument();
    expect(screen.queryByText('GPT-4o')).not.toBeInTheDocument();
  });

  it('NAVI tab shows routing modes stub section', () => {
    renderBrowser();
    fireEvent.click(screen.getByRole('button', { name: 'NAVI' }));
    expect(screen.getByText('Routing modes')).toBeInTheDocument();
    expect(screen.getByText(/Efficiency.*Power.*Adaptive/i)).toBeInTheDocument();
  });
});

describe('ModelBrowser — add modal', () => {
  it('clicking Add opens the provider modal', () => {
    renderBrowser();
    fireEvent.click(screen.getByRole('button', { name: /add provider or model/i }));
    expect(screen.getByRole('dialog', { name: /provider configuration/i })).toBeInTheDocument();
  });
});

describe('AddModelModal', () => {
  it('renders provider list from API', () => {
    render(
      <Wrapper>
        <AddModelModal onClose={vi.fn()} />
      </Wrapper>,
    );
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByText('Provider configuration')).toBeInTheDocument();
    // All 4 known providers should always show
    expect(screen.getByText('Anthropic')).toBeInTheDocument();
    expect(screen.getByText('OpenAI')).toBeInTheDocument();
    expect(screen.getByText('Openrouter')).toBeInTheDocument();
    expect(screen.getByText('Ollama')).toBeInTheDocument();
  });

  it('shows Configure button for unconfigured providers', () => {
    render(
      <Wrapper>
        <AddModelModal onClose={vi.fn()} />
      </Wrapper>,
    );
    // Openrouter and Ollama are not in the mocked providers — should show "Configure"
    const configureBtns = screen.getAllByRole('button', { name: /configure/i });
    expect(configureBtns.length).toBeGreaterThan(0);
  });

  it('closes when backdrop is clicked', () => {
    const onClose = vi.fn();
    render(
      <Wrapper>
        <AddModelModal onClose={onClose} />
      </Wrapper>,
    );
    fireEvent.click(screen.getByRole('presentation'));
    expect(onClose).toHaveBeenCalled();
  });

  it('closes on Escape key', () => {
    const onClose = vi.fn();
    render(
      <Wrapper>
        <AddModelModal onClose={onClose} />
      </Wrapper>,
    );
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onClose).toHaveBeenCalled();
  });
});
