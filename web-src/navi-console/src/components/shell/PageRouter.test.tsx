import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';

// Control the route match.
let mockMatch: { route: string; params: Record<string, string>; path: string; query: URLSearchParams };
vi.mock('@/app/router', () => ({
  useRoute: () => mockMatch,
  useNavigate: () => vi.fn(),
}));

// Stub the two pages we assert on so the test does not depend on their data deps.
vi.mock('@/pages/plugin-detail/PluginDetailPage', () => ({
  PluginDetailPage: ({ pluginId }: { pluginId: string }) => <div data-testid="detail">detail:{pluginId}</div>,
}));
vi.mock('@/pages/PluginsPage', () => ({
  PluginsPage: () => <div data-testid="list">list</div>,
}));
vi.mock('@/pages/CeremonyPage', () => ({
  CeremonyPage: () => <div data-testid="ceremony">ceremony</div>,
}));
vi.mock('@/pages/CoderPage', () => ({
  CoderPage: ({ section }: { section?: string }) => <div data-testid="coder">coder:{section ?? 'workspace'}</div>,
}));

import { PageRouter } from './PageRouter';

function match(route: string, params: Record<string, string> = {}) {
  mockMatch = { route, params, path: '/' + route, query: new URLSearchParams() };
}

describe('PageRouter plugin routing', () => {
  beforeEach(() => match('chats'));

  it('routes /plugins/:id to the plugin detail view (regression guard)', () => {
    match('plugins', { section: 'navi.contacts' });
    render(<PageRouter />);
    expect(screen.getByTestId('detail')).toHaveTextContent('detail:navi.contacts');
  });

  it('routes bare /plugins to the list', () => {
    match('plugins', {});
    render(<PageRouter />);
    expect(screen.getByTestId('list')).toBeInTheDocument();
  });

  it('routes the reserved /plugins/skills sub-path to the list, not detail', () => {
    match('plugins', { section: 'skills' });
    render(<PageRouter />);
    expect(screen.getByTestId('list')).toBeInTheDocument();
    expect(screen.queryByTestId('detail')).toBeNull();
  });

  it('routes /ceremony to the Ceremony page', () => {
    match('ceremony');
    render(<PageRouter />);
    expect(screen.getByTestId('ceremony')).toBeInTheDocument();
  });

  it('routes /coder to the Coder workspace shell', () => {
    match('coder');
    render(<PageRouter />);
    expect(screen.getByTestId('coder')).toHaveTextContent('coder:workspace');
  });

  it('passes /coder/:section to the Coder workspace shell', () => {
    match('coder', { section: 'reviews' });
    render(<PageRouter />);
    expect(screen.getByTestId('coder')).toHaveTextContent('coder:reviews');
  });
});
