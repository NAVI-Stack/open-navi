import { describe, expect, it } from 'vitest';
import onboardingHTML from '../../../../web/onboarding.html?raw';

const normalizedOnboardingHTML = onboardingHTML.replace(/\r\n/g, '\n');

describe('static onboarding page', () => {
  it('uses console theme tokens and system color preference by default', () => {
    expect(onboardingHTML).toContain('--navi-bg');
    expect(onboardingHTML).toContain('--navi-panel');
    expect(onboardingHTML).toContain('--navi-accent');
    expect(onboardingHTML).toContain('@media (prefers-color-scheme: light)');
    expect(onboardingHTML).toContain('function hydrateAppearance()');
  });

  it('keeps first-run setup API calls intact', () => {
    expect(onboardingHTML).toContain('/api/onboarding/status');
    expect(onboardingHTML).toContain('/api/onboarding/recovery');
    expect(onboardingHTML).toContain('/api/onboarding/provider');
    expect(onboardingHTML).toContain('/api/onboarding/connection');
    expect(onboardingHTML).toContain('/api/onboarding/complete');
  });

  it('branches recovery save UX by runtime mode', () => {
    expect(onboardingHTML).toContain('runtime_mode');
    expect(onboardingHTML).toContain('showSaveFilePicker');
    expect(onboardingHTML).toContain('Choose save location');
    expect(onboardingHTML).toContain('Download passport');
  });

  it('stacks provider selection above model selection', () => {
    expect(normalizedOnboardingHTML).toContain('<div class="stack">\n          <label>Provider');
    expect(normalizedOnboardingHTML).toContain('<div id="model-field">');
  });
});
