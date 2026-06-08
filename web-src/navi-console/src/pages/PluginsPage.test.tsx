import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

const navigate = vi.fn();
vi.mock('@/app/router', () => ({ useNavigate: () => navigate }));

const pluginsState = { data: { plugins: [] as Array<Record<string, unknown>> }, isLoading: false };
const skillsState = { data: { items: [] as Array<Record<string, unknown>> }, isLoading: false };
vi.mock('@/api/extensions', () => ({
  usePlugins: () => pluginsState,
  useSkills: () => skillsState,
}));

import { PluginsPage } from './PluginsPage';

describe('PluginsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    pluginsState.data = {
      plugins: [
        { id: 'navi.contacts', name: 'NAVI Contacts', version: '1.0.0', kind: 'domain', active: true, metadata: { description: 'Contacts' } },
      ],
    };
    skillsState.data = { items: [] };
  });

  it('renders plugin cards and navigates to the detail route on click (list→detail entry point)', () => {
    render(<PluginsPage />);
    const card = screen.getByText('NAVI Contacts');
    fireEvent.click(card);
    expect(navigate).toHaveBeenCalledWith('/plugins/navi.contacts');
  });

  it('sorts plugins correctly by name, date added, and status', () => {
    pluginsState.data = {
      plugins: [
        { id: 'plugin-b', name: 'Plugin B', version: '1.0.0', kind: 'domain', active: false, date_added: '2026-05-20T12:00:00Z', metadata: { description: 'B' } },
        { id: 'plugin-a', name: 'Plugin A', version: '1.0.0', kind: 'domain', active: true, date_added: '2026-05-22T12:00:00Z', metadata: { description: 'A' } },
        { id: 'plugin-c', name: 'Plugin C', version: '1.0.0', kind: 'domain', active: true, date_added: '2026-05-21T12:00:00Z', metadata: { description: 'C' } },
      ],
    };
    
    const { container } = render(<PluginsPage />);
    
    // Default sorting is name-asc: A, B, C
    let cardTitles = screen.getAllByText(/Plugin [A-C]/);
    expect(cardTitles[0].textContent).toBe('Plugin A');
    expect(cardTitles[1].textContent).toBe('Plugin B');
    expect(cardTitles[2].textContent).toBe('Plugin C');
    
    // Sort by name-desc
    const select = container.querySelector('select');
    expect(select).toBeTruthy();
    if (select) {
      fireEvent.change(select, { target: { value: 'name-desc' } });
      cardTitles = screen.getAllByText(/Plugin [A-C]/);
      expect(cardTitles[0].textContent).toBe('Plugin C');
      expect(cardTitles[1].textContent).toBe('Plugin B');
      expect(cardTitles[2].textContent).toBe('Plugin A');

      // Sort by date-newest: A (May 22), C (May 21), B (May 20)
      fireEvent.change(select, { target: { value: 'date-newest' } });
      cardTitles = screen.getAllByText(/Plugin [A-C]/);
      expect(cardTitles[0].textContent).toBe('Plugin A');
      expect(cardTitles[1].textContent).toBe('Plugin C');
      expect(cardTitles[2].textContent).toBe('Plugin B');

      // Sort by date-oldest: B (May 20), C (May 21), A (May 22)
      fireEvent.change(select, { target: { value: 'date-oldest' } });
      cardTitles = screen.getAllByText(/Plugin [A-C]/);
      expect(cardTitles[0].textContent).toBe('Plugin B');
      expect(cardTitles[1].textContent).toBe('Plugin C');
      expect(cardTitles[2].textContent).toBe('Plugin A');

      // Sort by status-active: Active first (A, C), inactive last (B). Secondary sort is name-asc: A, C, B
      fireEvent.change(select, { target: { value: 'status-active' } });
      cardTitles = screen.getAllByText(/Plugin [A-C]/);
      expect(cardTitles[0].textContent).toBe('Plugin A');
      expect(cardTitles[1].textContent).toBe('Plugin C');
      expect(cardTitles[2].textContent).toBe('Plugin B');

      // Sort by status-inactive: Inactive first (B), active last (A, C). Secondary sort is name-asc: B, A, C
      fireEvent.change(select, { target: { value: 'status-inactive' } });
      cardTitles = screen.getAllByText(/Plugin [A-C]/);
      expect(cardTitles[0].textContent).toBe('Plugin B');
      expect(cardTitles[1].textContent).toBe('Plugin A');
      expect(cardTitles[2].textContent).toBe('Plugin C');
    }
  });
});
