import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { GenUI, naviSpecEngine, parseNaviSpec, createOpenUIEngine } from './index';

describe('NaviSpecEngine', () => {
  it('parses a valid node', () => {
    const res = parseNaviSpec({ t: 'text', value: 'hi' });
    expect(res.ok).toBe(true);
  });

  it('accepts a JSON string and an { root } envelope', () => {
    const res = parseNaviSpec(JSON.stringify({ root: { t: 'badge', label: 'X' } }));
    expect(res.ok).toBe(true);
    if (res.ok) expect(res.root.t).toBe('badge');
  });

  it('rejects an unknown node type', () => {
    const res = parseNaviSpec({ t: 'nope' });
    expect(res.ok).toBe(false);
  });

  it('rejects malformed JSON without throwing', () => {
    const res = parseNaviSpec('{ not json');
    expect(res.ok).toBe(false);
  });

  it('describes its vocabulary in the system prompt', () => {
    expect(naviSpecEngine.systemPrompt()).toContain('NAVI UI Spec');
  });
});

describe('createOpenUIEngine (adapter seam)', () => {
  it('is inert but reports a clear error when no adapter is provided', () => {
    const engine = createOpenUIEngine();
    const res = engine.parse('anything');
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.error).toMatch(/@openuidev\/react-lang/);
  });

  it('uses the adapter and re-validates its output', () => {
    const engine = createOpenUIEngine({ toNaviSpec: () => ({ t: 'text', value: 'from openui' }) });
    const res = engine.parse('<ignored OpenUI Lang>');
    expect(res.ok).toBe(true);
  });
});

describe('<GenUI>', () => {
  it('renders text from a spec', () => {
    render(<GenUI source={{ t: 'text', value: 'Hello GenUI' }} />);
    expect(screen.getByText('Hello GenUI')).toBeInTheDocument();
  });

  it('renders nested cards, stacks and badges', () => {
    render(
      <GenUI
        source={{
          t: 'card',
          title: 'Status',
          children: [{ t: 'stack', dir: 'row', children: [{ t: 'badge', label: 'Running', status: 'running' }] }],
        }}
      />,
    );
    expect(screen.getByText('Status')).toBeInTheDocument();
    expect(screen.getByText('Running')).toBeInTheDocument();
  });

  it('renders a visible error (not a crash) for an invalid spec', () => {
    render(<GenUI source={{ t: 'totally-unknown' }} />);
    expect(screen.getByRole('alert')).toBeInTheDocument();
  });
});
