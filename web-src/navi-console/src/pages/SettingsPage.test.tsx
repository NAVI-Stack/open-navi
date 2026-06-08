import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

vi.mock('./Config', () => ({
  Config: () => <div>system config</div>,
}));
vi.mock('./Appearance', () => ({
  Appearance: () => <div>appearance config</div>,
}));
vi.mock('./Workspaces', () => ({
  Workspaces: () => <div>workspace config</div>,
}));

import { SettingsPage } from './SettingsPage';

describe('SettingsPage relationship entry', () => {
  it('exposes a NAVI relationship entry point back into Ceremony-derived choices', () => {
    render(<SettingsPage />);

    fireEvent.click(screen.getByRole('button', { name: /NAVI relationship/i }));

    expect(screen.getByRole('heading', { name: /NAVI relationship/i })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /Adjust relationship choices/i })).toHaveAttribute(
      'href',
      '/ceremony?mode=adjust',
    );
  });
});
