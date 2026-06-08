import { describe, expect, it } from 'vitest';
import { buildCeremonyStepRequestBody } from './ceremony';

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
