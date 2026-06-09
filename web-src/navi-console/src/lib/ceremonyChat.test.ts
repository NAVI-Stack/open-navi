import { describe, expect, it } from 'vitest';
import { shouldShowCeremonyControls } from './ceremonyChat';

describe('shouldShowCeremonyControls', () => {
  it('shows controls only for the active ceremony step', () => {
    expect(
      shouldShowCeremonyControls(true, 'pact_summary', 'pact_summary'),
    ).toBe(true);
    expect(
      shouldShowCeremonyControls(true, 'trust_boundaries', 'pact_summary'),
    ).toBe(false);
  });

  it('hides controls after ceremony completes', () => {
    expect(
      shouldShowCeremonyControls(false, 'pact_summary', 'pact_summary'),
    ).toBe(false);
  });

  it('hides controls when current step is missing', () => {
    expect(shouldShowCeremonyControls(true, 'pact_summary', undefined)).toBe(
      false,
    );
  });
});
