import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { applyTheme, createDefaultTheme, type ThemeModel } from './theme';

interface ThemeContextValue {
  theme: ThemeModel;
  setTheme: (theme: ThemeModel) => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

/** Access the active theme and a setter. Must be used within <ThemeProvider>. */
export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme must be used within a ThemeProvider');
  return ctx;
}

export interface ThemeProviderProps {
  /** Initial theme. Defaults to the NAVI default theme. */
  theme?: ThemeModel;
  /** Element to project CSS variables onto. Defaults to `#root`. */
  target?: HTMLElement | null;
  children: ReactNode;
}

/**
 * Applies a ThemeModel to the DOM as CSS custom properties and exposes it via
 * context. Apps that manage their own persistence can drive `theme` from props;
 * standalone apps can rely on the default and `setTheme`.
 */
export function ThemeProvider({ theme: initialTheme, target, children }: ThemeProviderProps) {
  const [theme, setTheme] = useState<ThemeModel>(() => initialTheme ?? createDefaultTheme());

  // Keep internal state in sync when a controlled `theme` prop changes.
  useEffect(() => {
    if (initialTheme) setTheme(initialTheme);
  }, [initialTheme]);

  useEffect(() => {
    applyTheme(theme, target);
  }, [theme, target]);

  const value = useMemo<ThemeContextValue>(() => ({ theme, setTheme }), [theme]);

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}
