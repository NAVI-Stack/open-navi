import { describe, expect, it } from 'vitest';
import { buildCeremonyStepRequestBody, CeremonyStepResponseSchema } from './ceremony';

describe('buildCeremonyStepRequestBody', () => {
  it('sends action (not value) for pact_summary', () => {
    expect(
      buildCeremonyStepRequestBody({
        chatId: 'chat-1',
        step: 'pact_summary',
        value: 'confirm',
      }),
    ).toEqual({
      chatId: 'chat-1',
      step: 'pact_summary',
      action: 'confirm',
    });
  });

  it('prefers explicit action for pact_summary', () => {
    expect(
      buildCeremonyStepRequestBody({
        chatId: 'chat-1',
        step: 'pact_summary',
        value: 'skip',
        action: 'confirm',
      }),
    ).toEqual({
      chatId: 'chat-1',
      step: 'pact_summary',
      action: 'confirm',
    });
  });

  it('parses pact confirm responses without redirect', () => {
    const parsed = CeremonyStepResponseSchema.parse({
      step: 'pact_summary',
      action: 'confirm',
      ok: true,
    });
    expect(parsed.ok).toBe(true);
    expect(parsed.redirect).toBeUndefined();
  });

  it('sends value for structured choice steps', () => {
    expect(
      buildCeremonyStepRequestBody({
        chatId: 'chat-1',
        step: 'navi_presence',
        value: 'balanced',
      }),
    ).toEqual({
      chatId: 'chat-1',
      step: 'navi_presence',
      value: 'balanced',
    });
  });
});
