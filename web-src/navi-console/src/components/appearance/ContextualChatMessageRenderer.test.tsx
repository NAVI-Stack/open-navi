import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ContextualChatMessageRenderer } from './ContextualChatMessageRenderer';
import { createDefaultAppearance, type ConsoleAppearanceState } from '@/appearance/theme';

const MOCK_SAVE_MUTATE = vi.fn();

vi.mock('@/api/appearance', () => ({
  useSaveConsoleAppearance: () => ({
    mutateAsync: MOCK_SAVE_MUTATE,
    isPending: false,
  }),
}));

describe('ContextualChatMessageRenderer', () => {
  let appearance: ConsoleAppearanceState;

  beforeEach(() => {
    vi.clearAllMocks();
    appearance = createDefaultAppearance();
  });

  it('renders a plain user message', () => {
    render(
      <ContextualChatMessageRenderer
        message={{ id: '1', role: 'user', parts: [{ type: 'text', text: 'hello' }] } as any}
        currentAppearance={appearance}
        onPreviewPatch={vi.fn()}
        onApplyPatch={vi.fn()}
      />
    );
    expect(screen.getByText('hello')).toBeInTheDocument();
    expect(screen.queryByText('NAVI')).toBeNull();
  });

  it('renders assistant message text', () => {
    render(
      <ContextualChatMessageRenderer
        message={{ id: '1', role: 'assistant', parts: [{ type: 'text', text: 'I am helping' }] } as any}
        currentAppearance={appearance}
        onPreviewPatch={vi.fn()}
        onApplyPatch={vi.fn()}
      />
    );
    expect(screen.getByText('NAVI')).toBeInTheDocument();
    expect(screen.getByText('I am helping')).toBeInTheDocument();
  });

  it('detects a valid theme proposal and renders the preview card', () => {
    const content = `Here is a dark theme.
\`\`\`json
{
  "type": "theme-proposal",
  "version": 1,
  "rationale": "Switching to dark mode with a blue accent.",
  "patch": {
    "mode": "dark",
    "accent": "#3b82f6"
  }
}
\`\`\``;
    
    render(
      <ContextualChatMessageRenderer
        message={{ id: '1', role: 'assistant', parts: [{ type: 'text', text: content }] } as any}
        currentAppearance={appearance}
        onPreviewPatch={vi.fn()}
        onApplyPatch={vi.fn()}
      />
    );
    
    expect(screen.getByText('Here is a dark theme.')).toBeInTheDocument();
    expect(screen.getByText('Theme Proposal')).toBeInTheDocument();
    expect(screen.getByText('Switching to dark mode with a blue accent.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /preview/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /apply/i })).toBeInTheDocument();
  });

  it('rejects invalid JSON schemas silently (renders as raw text without card)', () => {
    const content = `Oops.
\`\`\`json
{
  "type": "theme-proposal",
  "version": 999,
  "rationale": "Invalid version",
  "patch": { "malicious_script": "alert(1)" }
}
\`\`\``;

    render(
      <ContextualChatMessageRenderer
        message={{ id: '1', role: 'assistant', parts: [{ type: 'text', text: content }] } as any}
        currentAppearance={appearance}
        onPreviewPatch={vi.fn()}
        onApplyPatch={vi.fn()}
      />
    );

    // No proposal card should appear since validation fails
    expect(screen.queryByText('Theme Proposal')).toBeNull();
  });

  it('triggers onPreviewPatch when preview is clicked, and can revert', () => {
    const onPreviewPatch = vi.fn();
    const content = `\`\`\`json
{
  "type": "theme-proposal",
  "version": 1,
  "rationale": "Previewing dark mode.",
  "patch": { "mode": "dark" }
}
\`\`\``;

    render(
      <ContextualChatMessageRenderer
        message={{ id: '1', role: 'assistant', parts: [{ type: 'text', text: content }] } as any}
        currentAppearance={appearance}
        onPreviewPatch={onPreviewPatch}
        onApplyPatch={vi.fn()}
      />
    );

    const previewBtn = screen.getByRole('button', { name: /preview/i });
    fireEvent.click(previewBtn);

    expect(onPreviewPatch).toHaveBeenCalledTimes(1);
    const passedTheme = onPreviewPatch.mock.calls[0][0];
    expect(passedTheme.theme.mode).toBe('dark');

    // The button should now be "Revert"
    const revertBtn = screen.getByRole('button', { name: /revert/i });
    fireEvent.click(revertBtn);

    expect(onPreviewPatch).toHaveBeenCalledTimes(2);
    expect(onPreviewPatch.mock.calls[1][0]).toBeNull();
  });

  it('triggers onApplyPatch and persistence via useSaveConsoleAppearance', async () => {
    const onApplyPatch = vi.fn();
    MOCK_SAVE_MUTATE.mockResolvedValueOnce({ appearance });
    
    const content = `\`\`\`json
{
  "type": "theme-proposal",
  "version": 1,
  "rationale": "Applying accent.",
  "patch": { "accent": "#ff0000" }
}
\`\`\``;

    render(
      <ContextualChatMessageRenderer
        message={{ id: '1', role: 'assistant', parts: [{ type: 'text', text: content }] } as any}
        currentAppearance={appearance}
        onPreviewPatch={vi.fn()}
        onApplyPatch={onApplyPatch}
      />
    );

    const applyBtn = screen.getByRole('button', { name: /apply/i });
    fireEvent.click(applyBtn);

    expect(onApplyPatch).toHaveBeenCalledTimes(1);
    expect(onApplyPatch.mock.calls[0][0].theme.accent).toBe('#ff0000');
    expect(MOCK_SAVE_MUTATE).toHaveBeenCalledTimes(1);
    
    await waitFor(() => {
      expect(MOCK_SAVE_MUTATE).toHaveBeenCalledWith(onApplyPatch.mock.calls[0][0]);
    });
  });
});
