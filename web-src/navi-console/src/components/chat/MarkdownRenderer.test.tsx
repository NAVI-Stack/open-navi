import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MarkdownRenderer } from './MarkdownRenderer';

describe('MarkdownRenderer', () => {
  beforeEach(() => vi.clearAllMocks());

  it('renders GFM markdown (bold, lists, inline code)', () => {
    render(<MarkdownRenderer content={'**bold** and `inline`\n\n- one\n- two'} />);
    expect(screen.getByText('bold', { selector: 'strong' })).toBeInTheDocument();
    expect(screen.getByText('inline', { selector: 'code' })).toBeInTheDocument();
    expect(screen.getByText('one')).toBeInTheDocument();
    expect(screen.getByText('two')).toBeInTheDocument();
  });

  it('renders fenced code blocks with a working copy button', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });

    render(<MarkdownRenderer content={'```js\nconst x = 1;\n```'} />);
    const copyBtn = screen.getByRole('button', { name: /copy code/i });
    fireEvent.click(copyBtn);
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('const x = 1;'));
  });
});
