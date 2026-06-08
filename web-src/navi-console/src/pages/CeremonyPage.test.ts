import { describe, expect, it } from 'vitest';
import { buildPactSummary } from './CeremonyPage';

describe('buildPactSummary', () => {
  it('does not duplicate punctuation from owner-provided seed text', () => {
    const lines = buildPactSummary(
      'Eric',
      'direct_strategic',
      { confirm_before_changing_files: true },
      'I prefer direct answers.',
    );

    expect(lines).toContain("I'll remember I prefer direct answers.");
    expect(lines).not.toContain("I'll remember I prefer direct answers..");
  });

  it('generates a pact summary with the correct name and presence mode', () => {
    const lines = buildPactSummary('Alex', 'calm_quiet', {}, '');
    expect(lines.some((l) => l.includes("I'll call you Alex"))).toBe(true);
    expect(lines.some((l) => l.toLowerCase().includes('calm'))).toBe(true);
  });

  it('omits the remember line when no personalization seed provided', () => {
    const lines = buildPactSummary('Sam', 'direct_strategic', {}, '');
    expect(lines.every((l) => !l.startsWith("I'll remember"))).toBe(true);
  });

  it('includes the remember line when a personalization seed is provided', () => {
    const lines = buildPactSummary('Sam', 'direct_strategic', {}, 'Keep answers short.');
    expect(lines.some((l) => l.startsWith("I'll remember"))).toBe(true);
  });
});

describe('CeremonyPage visible title constraints', () => {
  it('VISIBLE_STEPS does not appear in the pact summary output', () => {
    // The wizard step labels (Name/Presence/Trust/Remember/Pact) must NOT
    // leak into any user-facing output from the shared utility functions.
    const WIZARD_STEP_LABELS = ['Name', 'Presence', 'Trust', 'Remember', 'Pact'];
    const lines = buildPactSummary('Eric', 'direct_strategic', {}, 'Short answers please');
    for (const label of WIZARD_STEP_LABELS) {
      expect(lines.every((l) => !l.includes(label))).toBe(true);
    }
  });

  it('pact summary does not contain the string "NAVI Ceremony"', () => {
    const lines = buildPactSummary('Eric', 'balanced', {}, '');
    expect(lines.every((l) => !l.includes('NAVI Ceremony'))).toBe(true);
  });
});
