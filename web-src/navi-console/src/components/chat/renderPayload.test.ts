import { describe, it, expect } from 'vitest';
import { parseRenderPayload } from './renderPayload';

describe('parseRenderPayload', () => {
  it('parses a well-formed openui render payload from metadata', () => {
    const meta = {
      renderPayload: {
        mode: 'openui',
        openuiLang: 'root = Card("x", [])',
        fallbackMarkdown: '### Tool usage',
        dataView: { id: 'tool-usage', title: 'Tool usage', intent: 'chart', dataset: { columns: [], rows: [] } },
      },
    };
    const payload = parseRenderPayload(meta);
    expect(payload).not.toBeNull();
    expect(payload?.mode).toBe('openui');
    expect(payload?.openuiLang).toContain('root = Card');
    expect(payload?.fallbackMarkdown).toBe('### Tool usage');
  });

  it('returns null when there is no render payload', () => {
    expect(parseRenderPayload({})).toBeNull();
    expect(parseRenderPayload({ toolParts: [] })).toBeNull();
    expect(parseRenderPayload(undefined)).toBeNull();
    expect(parseRenderPayload(null)).toBeNull();
  });

  it('returns null when the payload lacks a string mode', () => {
    expect(parseRenderPayload({ renderPayload: {} })).toBeNull();
    expect(parseRenderPayload({ renderPayload: { mode: 123 } })).toBeNull();
  });

  it('preserves prototype and dataSourceKind fields on an openui prototype payload', () => {
    const meta = {
      renderPayload: {
        mode: 'openui',
        openuiLang: 'root = Card("Connector Health Dashboard", [])',
        dataSourceKind: 'placeholder',
        prototype: {
          id: 'prototype-dashboard',
          title: 'Connector Health Dashboard',
          purpose: 'dashboard',
          dataSourceKind: 'placeholder',
        },
        fallbackMarkdown: '### Connector Health Dashboard (prototype)',
      },
    };
    const payload = parseRenderPayload(meta);
    expect(payload).not.toBeNull();
    expect(payload?.mode).toBe('openui');
    expect(payload?.dataSourceKind).toBe('placeholder');
    expect(payload?.prototype?.title).toBe('Connector Health Dashboard');
    expect(payload?.prototype?.purpose).toBe('dashboard');
  });
});
