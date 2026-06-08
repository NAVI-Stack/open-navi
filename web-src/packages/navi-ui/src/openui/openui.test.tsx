import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { GenUI } from '../genui';
import { createNaviOpenUIEngine, createNaviOpenUIAdapter, mapOpenUIElement, naviOpenUISystemPrompt } from './index';

const SAMPLE = [
  'root = Card("Create project", [intro, form, tags])',
  'intro = Text("Name your project.", "secondary")',
  'form = Form("create_project", [titleField], "Create")',
  'titleField = Field("title", "Project name", "e.g. Apollo")',
  'tags = Stack([act, draft], "row", "sm")',
  'act = Badge("ACT", "running")',
  'draft = Badge("draft", "idle")',
].join('\n');

describe('OpenUI adapter (@navi/ui/openui)', () => {
  it('parses a full OpenUI Lang program into a NAVI node tree', () => {
    const engine = createNaviOpenUIEngine();
    const res = engine.parse(SAMPLE);
    expect(res.ok).toBe(true);
    if (res.ok) {
      expect(res.root.t).toBe('card');
      if (res.root.t === 'card') expect(res.root.children.length).toBe(3);
    }
  });

  it('renders OpenUI Lang through NAVI primitives via <GenUI>', () => {
    const engine = createNaviOpenUIEngine();
    render(<GenUI engine={engine} source={SAMPLE} />);
    expect(screen.getByText('Create project')).toBeInTheDocument();
    expect(screen.getByText('Name your project.')).toBeInTheDocument();
    expect(screen.getByText('ACT')).toBeInTheDocument();
    expect(screen.getByText('Project name')).toBeInTheDocument();
  });

  it('maps a Button intent into a declarative NAVI action', () => {
    const adapter = createNaviOpenUIAdapter();
    const node = adapter.toNaviSpec('root = Card("x", [b])\nb = Button("Go", "primary", "do_it")');
    // root is a card; the button is its child.
    expect(node.t).toBe('card');
    if (node.t === 'card') {
      const btn = node.children.find((c) => c.t === 'button');
      expect(btn).toBeTruthy();
      if (btn && btn.t === 'button') {
        expect(btn.action?.intent).toBe('do_it');
        expect(btn.variant).toBe('primary');
      }
    }
  });

  it('maps a synthetic ElementNode directly', () => {
    const mapped = mapOpenUIElement({
      type: 'element',
      typeName: 'Badge',
      props: { label: 'Done', status: 'completed' },
      partial: false,
    });
    expect(mapped).toEqual({ t: 'badge', label: 'Done', status: 'completed' });
  });

  it('renders the valid prefix of a truncated (streaming) program', () => {
    const engine = createNaviOpenUIEngine();
    // Cut the program partway through — the shell + early children should survive.
    const partialText = SAMPLE.slice(0, SAMPLE.indexOf('tags ='));
    const res = engine.parsePartial!(partialText);
    expect(res.ok).toBe(true);
    if (res.ok) {
      expect(res.partial).toBe(true);
      expect(res.root.t).toBe('card');
    }
  });

  it('emits a grammar-accurate openui-lang system prompt', () => {
    const prompt = naviOpenUISystemPrompt();
    expect(prompt).toContain('openui-lang');
    expect(prompt).toContain('root = Card');
    expect(prompt).toContain('POSITIONAL');
  });
});
