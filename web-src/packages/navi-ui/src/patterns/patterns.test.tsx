import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { Dialog, ToastProvider, useToast, ConfirmProvider, useConfirm } from './index';

describe('Dialog primitive', () => {
  it('renders title + body when open', () => {
    render(
      <Dialog isOpen onOpenChange={() => {}} title="Heads up">
        Body content here
      </Dialog>,
    );
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByText('Heads up')).toBeInTheDocument();
    expect(screen.getByText('Body content here')).toBeInTheDocument();
  });

  it('renders nothing when closed', () => {
    render(
      <Dialog isOpen={false} onOpenChange={() => {}} title="Hidden">
        nope
      </Dialog>,
    );
    expect(screen.queryByText('Hidden')).toBeNull();
  });
});

function ToastTrigger() {
  const { success } = useToast();
  return <button onClick={() => success('Saved!')}>fire</button>;
}

describe('Toast', () => {
  it('shows a toast message via the provider', async () => {
    render(
      <ToastProvider>
        <ToastTrigger />
      </ToastProvider>,
    );
    fireEvent.click(screen.getByText('fire'));
    expect(await screen.findByText('Saved!')).toBeInTheDocument();
  });
});

function ConfirmTrigger({ onResult }: { onResult: (v: boolean) => void }) {
  const confirm = useConfirm();
  return <button onClick={async () => onResult(await confirm({ title: 'Delete this?', confirmLabel: 'Delete', danger: true }))}>ask</button>;
}

describe('ConfirmDialog', () => {
  it('opens an alertdialog and resolves true on confirm', async () => {
    let result: boolean | undefined;
    render(
      <ConfirmProvider>
        <ConfirmTrigger onResult={(v) => (result = v)} />
      </ConfirmProvider>,
    );
    fireEvent.click(screen.getByText('ask'));
    expect(await screen.findByText('Delete this?')).toBeInTheDocument();
    expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    fireEvent.click(screen.getByText('Delete'));
    await waitFor(() => expect(result).toBe(true));
  });
});
