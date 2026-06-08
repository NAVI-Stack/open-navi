import type { GlobalProvider } from '@ladle/react';
// Design tokens are defined on :root, so importing the stylesheet themes every
// story with the NAVI palette. The component CSS Modules are imported by the
// components themselves.
import '../src/tokens/tokens.css';

/** Wraps every story in a NAVI-themed surface. */
export const Provider: GlobalProvider = ({ children }) => (
  <div
    style={{
      background: 'var(--navi-bg)',
      color: 'var(--navi-text)',
      fontFamily: 'var(--navi-font-sans, system-ui, sans-serif)',
      minHeight: '100vh',
      padding: 'var(--navi-space-xl, 24px)',
      boxSizing: 'border-box',
    }}
  >
    {children}
  </div>
);
