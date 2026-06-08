import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { CeremonyControls } from './CeremonyControls';

const presenceOptions = [
  { key: 'calm_quiet', label: 'Calm and quiet' },
  { key: 'direct_strategic', label: 'Direct and strategic' },
];

const trustOptions = [
  { key: 'sending_messages', label: 'Sending messages' },
  { key: 'changing_files', label: 'Changing files' },
  { key: 'purchases', label: 'Making purchases or subscriptions' },
];

const pactActions = [
  { key: 'confirm', label: 'Looks right', variant: 'primary' as const },
  { key: 'adjust', label: 'Adjust', variant: 'secondary' as const },
  { key: 'skip', label: 'Skip for now', variant: 'ghost' as const },
];

describe('CeremonyControls — single_select', () => {
  it('renders presence option chips', () => {
    render(
      <CeremonyControls
        step="navi_presence"
        type="single_select"
        options={presenceOptions}
        onSelect={vi.fn()}
      />,
    );
    expect(screen.getByText('Calm and quiet')).toBeInTheDocument();
    expect(screen.getByText('Direct and strategic')).toBeInTheDocument();
  });

  it('calls onSelect with step and key when a chip is clicked', () => {
    const onSelect = vi.fn();
    render(
      <CeremonyControls
        step="navi_presence"
        type="single_select"
        options={presenceOptions}
        onSelect={onSelect}
      />,
    );
    fireEvent.click(screen.getByText('Direct and strategic'));
    expect(onSelect).toHaveBeenCalledWith('navi_presence', 'direct_strategic');
  });

  it('disables chips when disabled prop is true', () => {
    render(
      <CeremonyControls
        step="navi_presence"
        type="single_select"
        options={presenceOptions}
        onSelect={vi.fn()}
        disabled
      />,
    );
    for (const btn of screen.getAllByRole('button')) {
      expect(btn).toBeDisabled();
    }
  });
});

describe('CeremonyControls — multi_select', () => {
  it('renders checkboxes for each trust option', () => {
    render(
      <CeremonyControls
        step="trust_boundaries"
        type="multi_select"
        options={trustOptions}
        onSelect={vi.fn()}
      />,
    );
    expect(screen.getByText('Sending messages')).toBeInTheDocument();
    expect(screen.getByText('Changing files')).toBeInTheDocument();
    expect(screen.getAllByRole('checkbox')).toHaveLength(3);
  });

  it('pre-checks options matching defaults', () => {
    render(
      <CeremonyControls
        step="trust_boundaries"
        type="multi_select"
        options={trustOptions}
        defaults={{ sending_messages: true, changing_files: false }}
        onSelect={vi.fn()}
      />,
    );
    // Hidden native checkboxes carry the checked state.
    const checkboxes = screen.getAllByRole('checkbox') as HTMLInputElement[];
    expect(checkboxes[0].checked).toBe(true);
    expect(checkboxes[1].checked).toBe(false);
  });

  it('calls onSelect with a boolean map when Done is clicked', () => {
    const onSelect = vi.fn();
    render(
      <CeremonyControls
        step="trust_boundaries"
        type="multi_select"
        options={trustOptions}
        defaults={{ sending_messages: true }}
        onSelect={onSelect}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: /done/i }));
    expect(onSelect).toHaveBeenCalledWith('trust_boundaries', {
      sending_messages: true,
      changing_files: false,
      purchases: false,
    });
  });

  it('disables checkboxes and Done button when disabled', () => {
    render(
      <CeremonyControls
        step="trust_boundaries"
        type="multi_select"
        options={trustOptions}
        onSelect={vi.fn()}
        disabled
      />,
    );
    for (const cb of screen.getAllByRole('checkbox')) {
      expect(cb).toBeDisabled();
    }
    expect(screen.getByRole('button', { name: /done/i })).toBeDisabled();
  });
});

describe('CeremonyControls — actions', () => {
  it('renders pact summary action buttons', () => {
    render(
      <CeremonyControls
        step="pact_summary"
        type="actions"
        actions={pactActions}
        onSelect={vi.fn()}
      />,
    );
    expect(screen.getByText('Looks right')).toBeInTheDocument();
    expect(screen.getByText('Adjust')).toBeInTheDocument();
    expect(screen.getByText('Skip for now')).toBeInTheDocument();
  });

  it('calls onSelect with step and action key when clicked', () => {
    const onSelect = vi.fn();
    render(
      <CeremonyControls
        step="pact_summary"
        type="actions"
        actions={pactActions}
        onSelect={onSelect}
      />,
    );
    fireEvent.click(screen.getByText('Looks right'));
    expect(onSelect).toHaveBeenCalledWith('pact_summary', 'confirm');
  });

  it('disables all action buttons when disabled', () => {
    render(
      <CeremonyControls
        step="pact_summary"
        type="actions"
        actions={pactActions}
        onSelect={vi.fn()}
        disabled
      />,
    );
    for (const btn of screen.getAllByRole('button')) {
      expect(btn).toBeDisabled();
    }
  });
});

describe('CeremonyControls — no type', () => {
  it('renders nothing when type is undefined', () => {
    const { container } = render(
      <CeremonyControls step="navi_presence" onSelect={vi.fn()} />,
    );
    expect(container.firstChild).toBeNull();
  });
});
