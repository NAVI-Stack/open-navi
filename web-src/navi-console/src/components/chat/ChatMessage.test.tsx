import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ChatMessage } from './ChatMessage';

function mockClipboard() {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
  return writeText;
}

describe('ChatMessage', () => {
  beforeEach(() => vi.clearAllMocks());

  it('renders an assistant message with the NAVI label and prose content', () => {
    render(<ChatMessage message={{ id: 'a1', role: 'assistant', content: 'Hello from **the agent**' }} />);
    expect(screen.getByText('NAVI')).toBeInTheDocument();
    // Markdown bold renders the inner text.
    expect(screen.getByText('the agent', { selector: 'strong' })).toBeInTheDocument();
  });

  it('renders a strict NAVI UI spec assistant payload inline', () => {
    const content = JSON.stringify({
      t: 'card',
      title: 'Run summary',
      children: [{ t: 'text', value: '3 checks passed', tone: 'success' }],
    });

    render(<ChatMessage message={{ id: 'a1', role: 'assistant', content }} />);

    expect(screen.getByLabelText('NAVI UI spec')).toBeInTheDocument();
    expect(screen.getByText('Run summary')).toBeInTheDocument();
    expect(screen.getByText('3 checks passed')).toBeInTheDocument();
  });

  it('uses the console markdown renderer for markdown nodes inside NAVI UI specs', () => {
    const content = JSON.stringify({
      t: 'markdown',
      value: 'Generated **markdown** node',
    });

    render(<ChatMessage message={{ id: 'a1', role: 'assistant', content }} />);

    expect(screen.getByLabelText('NAVI UI spec')).toBeInTheDocument();
    expect(screen.getByText('markdown', { selector: 'strong' })).toBeInTheDocument();
  });

  it('renders the valid prefix of a streaming NAVI UI spec assistant payload', () => {
    const fullSpec = JSON.stringify({
      t: 'card',
      title: 'Streaming panel',
      children: [{ t: 'text', value: 'still arriving' }],
    });
    const partialSpec = fullSpec.slice(0, 54);

    render(<ChatMessage message={{ id: 'a1', role: 'assistant', content: partialSpec, streaming: true }} />);

    expect(screen.getByLabelText('NAVI UI spec')).toBeInTheDocument();
    expect(screen.getByText('Streaming panel')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('shows a quiet pending state for an early streaming NAVI UI spec prefix', () => {
    render(<ChatMessage message={{ id: 'a1', role: 'assistant', content: '{"t"', streaming: true }} />);

    expect(screen.getByLabelText('NAVI UI spec')).toBeInTheDocument();
    expect(screen.getByText('Rendering UI...')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('falls back to markdown text for JSON that is not a NAVI UI spec', () => {
    const content = '{"kind":"not-ui","value":"keep me as text"}';

    render(<ChatMessage message={{ id: 'a1', role: 'assistant', content }} />);

    expect(screen.queryByLabelText('NAVI UI spec')).toBeNull();
    expect(screen.getByText(content)).toBeInTheDocument();
  });

  it('renders a user message without the NAVI role label', () => {
    render(<ChatMessage message={{ id: 'u1', role: 'user', content: 'hi there' }} />);
    expect(screen.queryByText('NAVI')).toBeNull();
    expect(screen.getByText('hi there')).toBeInTheDocument();
  });

  it('copies message content to the clipboard', async () => {
    const writeText = mockClipboard();
    render(<ChatMessage message={{ id: 'u1', role: 'user', content: 'copy me' }} />);
    fireEvent.click(screen.getByRole('button', { name: /copy message/i }));
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('copy me'));
  });

  it('shows a failed state with a working retry action', () => {
    const onRetry = vi.fn();
    render(
      <ChatMessage
        message={{ id: 'u1', role: 'user', content: 'oops', failed: true }}
        onRetry={onRetry}
      />,
    );
    expect(screen.getByText('Not sent')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /retry/i }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it('shows a streaming placeholder with no copy button until content arrives', () => {
    render(<ChatMessage message={{ id: 'a1', role: 'assistant', content: '', streaming: true }} />);
    expect(screen.queryByRole('button', { name: /copy message/i })).toBeNull();
  });

  it('renders feedback thumbs on assistant messages and reports the rating', () => {
    const onFeedback = vi.fn();
    render(
      <ChatMessage
        message={{ id: 'a1', role: 'assistant', content: 'answer' }}
        feedback={null}
        onFeedback={onFeedback}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: /good response/i }));
    expect(onFeedback).toHaveBeenCalledWith('up');
  });

  it('toggles an active rating off when clicked again', () => {
    const onFeedback = vi.fn();
    render(
      <ChatMessage
        message={{ id: 'a1', role: 'assistant', content: 'answer' }}
        feedback={'down'}
        onFeedback={onFeedback}
      />,
    );
    const down = screen.getByRole('button', { name: /bad response/i });
    expect(down).toHaveAttribute('aria-pressed', 'true');
    fireEvent.click(down);
    expect(onFeedback).toHaveBeenCalledWith(null);
  });

  it('does not render feedback thumbs on user messages', () => {
    render(
      <ChatMessage
        message={{ id: 'u1', role: 'user', content: 'hi' }}
        onFeedback={vi.fn()}
      />,
    );
    expect(screen.queryByRole('button', { name: /good response/i })).toBeNull();
  });

  it('renders a regenerate control on assistant messages and invokes it', () => {
    const onRegenerate = vi.fn();
    render(
      <ChatMessage
        message={{ id: 'a1', role: 'assistant', content: 'answer' }}
        onRegenerate={onRegenerate}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: /regenerate response/i }));
    expect(onRegenerate).toHaveBeenCalledTimes(1);
  });

  it('renders a continue control on the last assistant message and invokes it', () => {
    const onContinue = vi.fn();
    render(
      <ChatMessage
        message={{ id: 'a1', role: 'assistant', content: 'answer' }}
        onContinue={onContinue}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: /continue response/i }));
    expect(onContinue).toHaveBeenCalledTimes(1);
  });

  it('renders a variant switcher and advances variants', () => {
    const onSelectVariant = vi.fn();
    render(
      <ChatMessage
        message={{
          id: 'a1',
          role: 'assistant',
          content: 'answer',
          variant: { selectedIndex: 0, total: 2 },
        }}
        onSelectVariant={onSelectVariant}
      />,
    );
    expect(screen.getByText('1/2')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /next variant/i }));
    expect(onSelectVariant).toHaveBeenCalledWith(1);
  });

  it('renders a running tool chip while a tool is in flight', () => {
    render(
      <ChatMessage
        message={{
          id: 'a1',
          role: 'assistant',
          content: '',
          streaming: true,
          toolParts: [{ toolInvocationId: 'c1', toolName: 'github.search', state: 'call' }],
        }}
      />,
    );
    expect(screen.getByText('github.search')).toBeInTheDocument();
    expect(screen.getByText('Running…')).toBeInTheDocument();
  });

  it('renders a resolved tool chip with its result preview', () => {
    render(
      <ChatMessage
        message={{
          id: 'a1',
          role: 'assistant',
          content: 'here you go',
          toolParts: [
            { toolInvocationId: 'c1', toolName: 'fs.read', state: 'result', result: 'found 3 repos' },
          ],
        }}
      />,
    );
    expect(screen.getByText('fs.read')).toBeInTheDocument();
    expect(screen.getByText('found 3 repos')).toBeInTheDocument();
    expect(screen.queryByText('Running…')).toBeNull();
  });

  it('renders a NAVI UI spec tool result under the resolved chip', () => {
    const result = JSON.stringify({
      t: 'card',
      title: 'Tool panel',
      children: [{ t: 'badge', label: 'Ready', status: 'completed' }],
    });

    render(
      <ChatMessage
        message={{
          id: 'a1',
          role: 'assistant',
          content: 'here you go',
          toolParts: [
            { toolInvocationId: 'c1', toolName: 'ui.render', state: 'result', result },
          ],
        }}
      />,
    );

    expect(screen.getByText('ui.render')).toBeInTheDocument();
    expect(screen.getByText('UI result')).toBeInTheDocument();
    expect(screen.getByLabelText('NAVI UI spec')).toBeInTheDocument();
    expect(screen.getByText('Tool panel')).toBeInTheDocument();
    expect(screen.getByText('Ready')).toBeInTheDocument();
  });

  it('renders an already-parsed NAVI UI spec tool result', () => {
    render(
      <ChatMessage
        message={{
          id: 'a1',
          role: 'assistant',
          content: 'structured result',
          toolParts: [
            {
              toolInvocationId: 'c1',
              toolName: 'ui.render',
              state: 'result',
              result: { t: 'text', value: 'Structured tool panel' },
            },
          ],
        }}
      />,
    );

    expect(screen.getByText('UI result')).toBeInTheDocument();
    expect(screen.getByText('Structured tool panel')).toBeInTheDocument();
  });

  it('marks a failed tool chip and does not show a result', () => {
    render(
      <ChatMessage
        message={{
          id: 'a1',
          role: 'assistant',
          content: 'sorry',
          toolParts: [
            { toolInvocationId: 'c1', toolName: 'fs.read', state: 'result', isError: true, result: 'Error: nope' },
          ],
        }}
      />,
    );
    expect(screen.getByText('Failed')).toBeInTheDocument();
  });

  it('renders no tool chips when there are none', () => {
    render(<ChatMessage message={{ id: 'a1', role: 'assistant', content: 'plain answer' }} />);
    expect(screen.queryByLabelText('Tool activity')).toBeNull();
  });

  it('edits a user message and submits the new content via onEditResend', () => {
    const onEditResend = vi.fn();
    render(
      <ChatMessage
        message={{ id: 'u1', role: 'user', content: 'original' }}
        onEditResend={onEditResend}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: /edit message/i }));
    const textarea = screen.getByRole('textbox', { name: /edit message/i });
    fireEvent.change(textarea, { target: { value: 'edited text' } });
    fireEvent.click(screen.getByRole('button', { name: /save & resend/i }));
    expect(onEditResend).toHaveBeenCalledWith('edited text');
  });

  // --- Data-driven render payload (OpenUI lane) ------------------------------

  const TOOL_USAGE_PROGRAM = [
    'root = Card("Tool usage in this chat", [metric, chart])',
    'metric = MetricCard("Total tool calls", "4", "across 1 tool")',
    'chart = UsageChart("Calls per tool", [b0])',
    'b0 = Bar("read_file", "4")',
  ].join('\n');

  it('renders the OpenUI data view and suppresses the text content on success', async () => {
    render(
      <ChatMessage
        message={{
          id: 'a1',
          role: 'assistant',
          content: 'PLAIN_FALLBACK_SENTINEL',
          renderPayload: {
            mode: 'openui',
            openuiLang: TOOL_USAGE_PROGRAM,
            fallbackMarkdown: 'FALLBACK_MARKDOWN_SENTINEL',
          },
        }}
      />,
    );
    // OpenUI engine loads lazily, then renders the NAVI domain components.
    await waitFor(() => expect(screen.getByText('Total tool calls')).toBeInTheDocument());
    expect(screen.getByText('Calls per tool')).toBeInTheDocument();
    // No duplicate rendering: neither the message text nor the markdown fallback show.
    expect(screen.queryByText('PLAIN_FALLBACK_SENTINEL')).toBeNull();
    expect(screen.queryByText('FALLBACK_MARKDOWN_SENTINEL')).toBeNull();
  });

  it('falls back to markdown when the OpenUI program is malformed', async () => {
    render(
      <ChatMessage
        message={{
          id: 'a1',
          role: 'assistant',
          content: 'RAW_CONTENT_SENTINEL',
          renderPayload: {
            mode: 'openui',
            openuiLang: 'this is not valid openui lang at all',
            fallbackMarkdown: 'Total tool calls: 4',
          },
        }}
      />,
    );
    await waitFor(() => expect(screen.getByText('Total tool calls: 4')).toBeInTheDocument());
    // The raw message content is not rendered alongside the fallback.
    expect(screen.queryByText('RAW_CONTENT_SENTINEL')).toBeNull();
  });

  it('renders a navi-ui data view directly from the canonical view', () => {
    render(
      <ChatMessage
        message={{
          id: 'a1',
          role: 'assistant',
          content: 'unused',
          renderPayload: {
            mode: 'navi-ui',
            fallbackMarkdown: 'fallback',
            dataView: {
              id: 'tool-usage',
              title: 'Tool usage in this chat',
              intent: 'chart',
              dataset: {
                columns: [
                  { key: 'tool', label: 'Tool', type: 'string' },
                  { key: 'count', label: 'Calls', type: 'number' },
                ],
                rows: [{ tool: 'read_file', count: 4 }],
              },
            },
          },
        }}
      />,
    );
    expect(screen.getByText('Tool usage in this chat')).toBeInTheDocument();
    expect(screen.getByText('Total tool calls')).toBeInTheDocument();
    expect(screen.queryByText('unused')).toBeNull();
  });

  // --- Generated UI prototype render payload (OpenUI lane, placeholder data) ---

  const PROTOTYPE_PROGRAM = [
    'root = Card("Connector Health Dashboard", [proto, banner, m1, act])',
    'proto = Badge("PROTOTYPE", "warning")',
    'banner = Text("Prototype with placeholder data — not live data, not saved.", "warning")',
    'm1 = MetricCard("Connectors", "4", "placeholder")',
    'act = Button("Approve", "primary", "approve_connector")',
  ].join('\n');

  const prototypePayload = {
    mode: 'openui' as const,
    openuiLang: PROTOTYPE_PROGRAM,
    dataSourceKind: 'placeholder' as const,
    prototype: {
      id: 'prototype-dashboard',
      title: 'Connector Health Dashboard',
      purpose: 'dashboard',
      dataSourceKind: 'placeholder' as const,
    },
    fallbackMarkdown: 'PROTO_FALLBACK_SENTINEL — prototype with placeholder data.',
  };

  it('renders an OpenUI prototype with a placeholder label and suppresses text on success', async () => {
    render(
      <ChatMessage
        message={{
          id: 'a1',
          role: 'assistant',
          content: 'PROTO_PLAIN_SENTINEL',
          renderPayload: prototypePayload,
        }}
      />,
    );
    // The prototype renders through the OpenUI lane (NAVI domain components).
    await waitFor(() => expect(screen.getByText('Connectors')).toBeInTheDocument());
    // Honest prototype chrome (chip) + the embedded PROTOTYPE badge are both visible.
    expect(screen.getByLabelText('Prototype with placeholder data')).toBeInTheDocument();
    expect(screen.getByText('PROTOTYPE')).toBeInTheDocument();
    // Neither the message text nor the markdown fallback are duplicated on success.
    expect(screen.queryByText('PROTO_PLAIN_SENTINEL')).toBeNull();
    expect(screen.queryByText(/PROTO_FALLBACK_SENTINEL/)).toBeNull();
  });

  it('falls back to honest prototype markdown when the OpenUI program is malformed', async () => {
    render(
      <ChatMessage
        message={{
          id: 'a1',
          role: 'assistant',
          content: 'PROTO_RAW_SENTINEL',
          renderPayload: {
            ...prototypePayload,
            openuiLang: 'this is not valid openui lang at all',
          },
        }}
      />,
    );
    await waitFor(() => expect(screen.getByText(/PROTO_FALLBACK_SENTINEL/)).toBeInTheDocument());
    // The placeholder label still shows alongside the fallback, and the raw text does not.
    expect(screen.getByLabelText('Prototype with placeholder data')).toBeInTheDocument();
    expect(screen.queryByText('PROTO_RAW_SENTINEL')).toBeNull();
  });

  it('labels the prototype render surface as a generated UI prototype (not a data view)', async () => {
    render(
      <ChatMessage
        message={{ id: 'a1', role: 'assistant', content: 'x', renderPayload: prototypePayload }}
      />,
    );
    await waitFor(() => expect(screen.getByText('Connectors')).toBeInTheDocument());
    expect(screen.getByLabelText('Generated UI prototype')).toBeInTheDocument();
    expect(screen.queryByLabelText('Data view')).toBeNull();
  });

  it('labels a real OpenUI data view surface as a data view (not a prototype)', async () => {
    render(
      <ChatMessage
        message={{
          id: 'a1',
          role: 'assistant',
          content: 'x',
          renderPayload: { mode: 'openui', openuiLang: TOOL_USAGE_PROGRAM, fallbackMarkdown: 'fb' },
        }}
      />,
    );
    await waitFor(() => expect(screen.getByText('Total tool calls')).toBeInTheDocument());
    expect(screen.getByLabelText('Data view')).toBeInTheDocument();
    expect(screen.queryByLabelText('Generated UI prototype')).toBeNull();
  });

  it('renders generated prototype buttons as inert (no privileged action)', async () => {
    render(
      <ChatMessage
        message={{ id: 'a1', role: 'assistant', content: 'x', renderPayload: prototypePayload }}
      />,
    );
    const button = await screen.findByRole('button', { name: 'Approve' });
    // It is a real, accessible button — not a link to a script/URL.
    expect(button.tagName).toBe('BUTTON');
    expect(button).not.toHaveAttribute('href');
    // Clicking is a no-op: no onAction is wired, so nothing throws or navigates and
    // the prototype surface stays mounted (the error boundary did not trip).
    fireEvent.click(button);
    expect(screen.getByTestId('openui-render')).toBeInTheDocument();
    expect(screen.getByText('Connectors')).toBeInTheDocument();
  });
});
