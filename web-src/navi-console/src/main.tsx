import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './app/App';
import { applyConsoleTheme, loadPersistedAppearance } from './appearance/theme';
import './styles/global.css';

const root = document.getElementById('root')!;

declare global {
  interface Window {
    __naviSplash?: {
      shimmerStarted: Promise<void>;
    };
  }
}

async function bootstrap() {
  try {
    const bootAppearance = await Promise.race([
      loadPersistedAppearance(),
      new Promise<null>((resolve) => setTimeout(() => resolve(null), 700)),
    ]);
    if (bootAppearance?.persisted) {
      applyConsoleTheme(bootAppearance.appearance.theme, root);
    }
  } catch {
    // The console can render with defaults if appearance hydration is unavailable.
  }

  createRoot(root).render(
    <StrictMode>
      <App />
    </StrictMode>,
  );

  const splash = document.getElementById('navi-splash-screen');
  if (splash) {
    const removeSplash = () => {
      splash.classList.add('fade-out');
      setTimeout(() => splash.remove(), 500);
    };

    if (window.__naviSplash?.shimmerStarted) {
      window.__naviSplash.shimmerStarted.then(() => {
        setTimeout(removeSplash, 400);
      });
    } else {
      removeSplash();
    }
  }
}

void bootstrap();
