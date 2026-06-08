import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useState, useEffect, useCallback } from 'react';
import { RouterProvider } from './router';
import { AppShell } from '@/components/shell/AppShell';
import { BootShell } from '@/components/shell/BootShell';
import { ToastProvider } from '@/components/ui/Toast';
import { ConfirmProvider } from '@/components/ui/ConfirmDialog';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 2,
      staleTime: 5_000,
      refetchOnWindowFocus: true,
    },
  },
});

type BootState = 'checking' | 'offline' | 'ready';

type OnboardingStatusForRouting = {
  complete?: boolean;
};

type CeremonyStatusForRouting = {
  journeyState?: {
    status?: string;
  };
  chatId?: string;
  redirect?: string;
} | null;

export function resolvePostSetupPath(
  onboarding: OnboardingStatusForRouting | null,
  ceremony: CeremonyStatusForRouting,
  currentPath: string,
): string | null {
  if (!onboarding?.complete) return '/onboarding';
  const status = ceremony?.journeyState?.status;
  if (status !== 'completed' && status !== 'skipped') {
    if (ceremony?.redirect) {
      const pathOnly = currentPath.split('?')[0] || '/';
      return pathOnly === ceremony.redirect ? null : ceremony.redirect;
    }
    // No chatId yet — checkSetup will call init-chat.
    return null;
  }
  return null;
}

function SetupGuard({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<BootState>('checking');

  const checkSetup = useCallback(async (signal?: AbortSignal) => {
    setState('checking');
    try {
      const res = await fetch('/api/onboarding/status', { signal });
      if (res.ok) {
        const data = await res.json();
        if (!data.complete) {
          const target = resolvePostSetupPath(data, null, window.location.pathname);
          if (target) window.location.href = target;
          return;
        }
        const ceremonyRes = await fetch('/api/ceremony', { signal, credentials: 'same-origin' });
        if (!ceremonyRes.ok) {
          setState('offline');
          return;
        }
        const ceremony = await ceremonyRes.json();
        const target = resolvePostSetupPath(data, ceremony, window.location.pathname + window.location.search);
        if (target) {
          window.location.href = target;
          return;
        }

        // Ceremony needs routing but has no chatId yet — create the Meet NAVI chat.
        const ceremonyStatus = ceremony?.journeyState?.status;
        if (ceremonyStatus !== 'completed' && ceremonyStatus !== 'skipped' && !ceremony?.redirect) {
          try {
            const initRes = await fetch('/api/ceremony/init-chat', {
              method: 'POST',
              credentials: 'same-origin',
              signal,
            });
            if (initRes.ok) {
              const initData = await initRes.json();
              if (initData.redirect) {
                const pathOnly = (window.location.pathname + window.location.search).split('?')[0] || '/';
                if (pathOnly !== initData.redirect.split('?')[0]) {
                  window.location.href = initData.redirect;
                  return;
                }
              }
            }
          } catch {
            // init-chat failed silently — fall through to ready state
          }
        }

        setState('ready');
        return;
      }
      // Backend reachable but returned an error status — treat as offline/unconfigured.
      setState('offline');
    } catch (err) {
      if (signal?.aborted) return;
      setState('offline');
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    checkSetup(controller.signal);
    return () => controller.abort();
  }, [checkSetup]);

  if (state === 'ready') return <>{children}</>;
  return <BootShell state={state} onRetry={() => checkSetup()} />;
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <ConfirmProvider>
          <SetupGuard>
            <RouterProvider>
              <AppShell />
            </RouterProvider>
          </SetupGuard>
        </ConfirmProvider>
      </ToastProvider>
    </QueryClientProvider>
  );
}
