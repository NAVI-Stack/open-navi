import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { GenUI, naviSpecEngine, parseNaviSpecPartial } from './index';
import { completeTruncatedJson, coerceValidPrefix } from './partial';

const FULL = {
  t: 'card',
  title: 'Status',
  children: [
    { t: 'text', value: 'Working on it' },
    { t: 'badge', label: 'Running', status: 'running' },
  ],
};

describe('completeTruncatedJson', () => {
  it('repairs a string truncated mid-value', () => {
    const json = '{"t":"card","title":"Status","children":[{"t":"text","value":"hel';
    const fixed = completeTruncatedJson(json);
    expect(fixed).not.toBeNull();
    expect(() => JSON.parse(fixed!)).not.toThrow();
  });

  it('repairs a truncation after a complete child + comma', () => {
    const json = '{"t":"stack","children":[{"t":"badge","label":"A"},';
    const fixed = completeTruncatedJson(json);
    const parsed = JSON.parse(fixed!);
    expect(parsed.t).toBe('stack');
    expect(Array.isArray(parsed.children)).toBe(true);
  });

  it('returns null for unrecoverable garbage', () => {
    expect(completeTruncatedJson('')).toBeNull();
  });
});

describe('coerceValidPrefix', () => {
  it('keeps the leading valid children and drops a half-streamed tail', () => {
    const root = coerceValidPrefix({
      t: 'card',
      children: [{ t: 'text', value: 'ok' }, { t: 'badge' /* missing label */ }],
    });
    expect(root).not.toBeNull();
    if (root && root.t === 'card') {
      expect(root.children.length).toBe(1);
      expect(root.children[0].t).toBe('text');
    }
  });

  it('returns null when nothing valid exists yet', () => {
    expect(coerceValidPrefix({ t: 'text' /* missing value */ })).toBeNull();
  });
});

describe('parseNaviSpecPartial', () => {
  it('parses a complete spec strictly (not flagged partial)', () => {
    const res = parseNaviSpecPartial(FULL);
    expect(res.ok).toBe(true);
    if (res.ok) expect(res.partial).toBeUndefined();
  });

  it('renders the valid prefix of a truncated JSON string', () => {
    const truncated = JSON.stringify(FULL).slice(0, 60); // cut mid-tree
    const res = parseNaviSpecPartial(truncated);
    expect(res.ok).toBe(true);
    if (res.ok) {
      expect(res.partial).toBe(true);
      expect(res.root.t).toBe('card');
    }
  });

  it('is exposed on the native engine', () => {
    expect(typeof naviSpecEngine.parsePartial).toBe('function');
  });
});

describe('<GenUI streaming>', () => {
  it('renders a partial spec without an error', () => {
    const truncated = JSON.stringify(FULL).slice(0, 60);
    render(<GenUI source={truncated} streaming />);
    // The card shell + its title survive even though the tree is incomplete.
    expect(screen.getByText('Status')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('without streaming, the same truncated spec shows a visible error', () => {
    const truncated = JSON.stringify(FULL).slice(0, 60);
    render(<GenUI source={truncated} />);
    expect(screen.getByRole('alert')).toBeInTheDocument();
  });
});
