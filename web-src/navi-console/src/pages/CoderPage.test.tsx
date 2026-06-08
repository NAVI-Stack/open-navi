import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

vi.mock('@/app/router', () => ({
  useNavigate: () => vi.fn(),
}));

import { CoderPage } from './CoderPage';

describe('CoderPage', () => {
  it('renders the NAVI Coder product shell and workspace sections', () => {
    render(<CoderPage />);

    expect(screen.getByRole('heading', { name: 'NAVI Coder' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Projects' })).toHaveAttribute('href', '/coder/projects');
    expect(screen.getByRole('link', { name: 'Repositories' })).toHaveAttribute('href', '/coder/repos');
    expect(screen.getByRole('link', { name: 'Tasks' })).toHaveAttribute('href', '/coder/tasks');
    expect(screen.getByRole('link', { name: 'Runs' })).toHaveAttribute('href', '/coder/runs');
    expect(screen.getByRole('link', { name: 'Reviews' })).toHaveAttribute('href', '/coder/reviews');
    expect(screen.getByRole('link', { name: 'Settings' })).toHaveAttribute('href', '/coder/settings');
  });

  it('does not introduce deprecated Programmer product language', () => {
    const { container } = render(<CoderPage />);

    expect(container).not.toHaveTextContent(/programmer/i);
  });
});
