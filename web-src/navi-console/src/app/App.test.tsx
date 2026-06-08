import { describe, expect, it } from 'vitest';
import { resolvePostSetupPath } from './App';

describe('resolvePostSetupPath', () => {
  it('routes incomplete required onboarding to /onboarding', () => {
    expect(resolvePostSetupPath({ complete: false }, null, '/')).toBe('/onboarding');
  });

  it('returns null when ceremony not started and no redirect yet (init-chat will be called)', () => {
    // No chatId, no redirect — checkSetup will call /api/ceremony/init-chat
    expect(resolvePostSetupPath(
      { complete: true },
      { journeyState: { status: 'not_started' } },
      '/',
    )).toBeNull();
  });

  it('routes to /chats/{chatId} when ceremony has a redirect', () => {
    expect(resolvePostSetupPath(
      { complete: true },
      { journeyState: { status: 'in_progress' }, redirect: '/chats/abc123' },
      '/',
    )).toBe('/chats/abc123');
  });

  it('does not redirect when already on the ceremony chat path', () => {
    expect(resolvePostSetupPath(
      { complete: true },
      { journeyState: { status: 'in_progress' }, redirect: '/chats/abc123' },
      '/chats/abc123',
    )).toBeNull();
  });

  it('allows the main app after ceremony is completed or skipped', () => {
    expect(resolvePostSetupPath(
      { complete: true },
      { journeyState: { status: 'completed' } },
      '/',
    )).toBeNull();
    expect(resolvePostSetupPath(
      { complete: true },
      { journeyState: { status: 'skipped' } },
      '/',
    )).toBeNull();
  });

  it('does not route to /ceremony', () => {
    const result = resolvePostSetupPath(
      { complete: true },
      { journeyState: { status: 'in_progress' }, redirect: '/chats/xyz' },
      '/',
    );
    expect(result).not.toBe('/ceremony');
  });
});
