import { describe, expect, it } from 'vitest';
import { render, act } from '@testing-library/react';
import { RouterProvider, useRoute, useNavigate } from './router';

// Probe surfaces the current route match and a navigate fn for assertions.
let current: ReturnType<typeof useRoute> | null = null;
let navigateFn: ReturnType<typeof useNavigate> | null = null;

function Probe() {
  current = useRoute();
  navigateFn = useNavigate();
  return null;
}

function setup() {
  window.history.pushState(null, '', '/');
  render(
    <RouterProvider>
      <Probe />
    </RouterProvider>,
  );
}

describe('router parsing', () => {
  it('parses /plugins/:section into a plugins route with a section param (the detail-route regression guard)', () => {
    setup();
    act(() => navigateFn!('/plugins/navi.contacts'));
    expect(current!.route).toBe('plugins');
    expect(current!.params.section).toBe('navi.contacts');
  });

  it('parses bare /plugins with no section', () => {
    setup();
    act(() => navigateFn!('/plugins'));
    expect(current!.route).toBe('plugins');
    expect(current!.params.section).toBeUndefined();
  });

  it('keeps the legacy /plugins/skills sub-path as a section', () => {
    setup();
    act(() => navigateFn!('/plugins/skills'));
    expect(current!.route).toBe('plugins');
    expect(current!.params.section).toBe('skills');
  });

  it('parses /coder as the Coder product route', () => {
    setup();
    act(() => navigateFn!('/coder'));
    expect(current!.route).toBe('coder');
    expect(current!.params.section).toBeUndefined();
  });

  it('parses /coder/:section as a Coder workspace section', () => {
    setup();
    act(() => navigateFn!('/coder/runs'));
    expect(current!.route).toBe('coder');
    expect(current!.params.section).toBe('runs');
  });
});
