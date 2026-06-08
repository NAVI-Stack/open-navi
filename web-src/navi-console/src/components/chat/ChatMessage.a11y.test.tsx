import { describe, it, expect } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { axe } from 'vitest-axe';
import { ChatMessage } from './ChatMessage';

describe('ChatMessage data-driven render — accessibility', () => {
  it('a rendered navi-ui data view has no axe violations', async () => {
    const { container } = render(
      <ChatMessage
        message={{
          id: 'a1',
          role: 'assistant',
          content: 'fallback',
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
                rows: [
                  { tool: 'read_file', count: 3 },
                  { tool: 'list_dir', count: 1 },
                ],
              },
            },
          },
        }}
      />,
    );
    expect((await axe(container)).violations).toEqual([]);
  });

  it('a rendered generated-UI prototype (placeholder data) has no axe violations', async () => {
    const { container } = render(
      <ChatMessage
        message={{
          id: 'a1',
          role: 'assistant',
          content: 'fallback',
          renderPayload: {
            mode: 'openui',
            dataSourceKind: 'placeholder',
            prototype: {
              id: 'prototype-dashboard',
              title: 'Connector Health Dashboard',
              purpose: 'dashboard',
              dataSourceKind: 'placeholder',
            },
            openuiLang: [
              'root = Card("Connector Health Dashboard", [proto, m1, act])',
              'proto = Badge("PROTOTYPE", "warning")',
              'm1 = MetricCard("Connectors", "4", "placeholder")',
              'act = Button("Approve", "primary", "approve_connector")',
            ].join('\n'),
            fallbackMarkdown: 'Connector Health Dashboard prototype with placeholder data.',
          },
        }}
      />,
    );
    // Wait for the lazy OpenUI engine to render the prototype, including the
    // React-Aria-backed button, before auditing accessibility.
    await waitFor(() => expect(screen.getByText('Connectors')).toBeInTheDocument());
    expect((await axe(container)).violations).toEqual([]);
  });
});
