import { describe, expect, it } from 'vitest';
import indexHtml from '../index.html?raw';

describe('console startup splash', () => {
  it('uses the PET-style splash shell with Navi SOLE branding', () => {
    expect(indexHtml).toContain('<link rel="stylesheet" href="/splash.css" />');
    expect(indexHtml).toContain('<link rel="stylesheet" href="/shimmer.css" />');
    expect(indexHtml).toContain('id="navi-splash-screen"');
    expect(indexHtml).toContain('<span class="navi-loader__visible">NAVI</span>');
    expect(indexHtml).toContain('<span class="navi-loader__hidden"><br />-SOLE</span>');
    expect(indexHtml).not.toContain('CONSOLE');
  });
});
