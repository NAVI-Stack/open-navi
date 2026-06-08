import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from 'react';
import { Dialog } from './Dialog';
import { Button } from '../primitives/Button';

export interface ConfirmOptions {
  title: string;
  description?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  /** Renders the confirm button in a destructive (danger) style. */
  danger?: boolean;
}

type ConfirmContextValue = (options: ConfirmOptions) => Promise<boolean>;

const ConfirmContext = createContext<ConfirmContextValue | null>(null);

// Fallback used when no ConfirmProvider is mounted (e.g. a component rendered in
// isolation under test). Degrades to the native confirm so destructive actions
// still require acknowledgement rather than silently proceeding.
const fallbackConfirm: ConfirmContextValue = (options) =>
  Promise.resolve(
    typeof window !== 'undefined'
      ? window.confirm(options.description ? `${options.title}\n\n${options.description}` : options.title)
      : false,
  );

export function useConfirm(): ConfirmContextValue {
  return useContext(ConfirmContext) ?? fallbackConfirm;
}

/**
 * Promise-based confirmation dialog. `useConfirm()(options)` resolves to the
 * user's choice. Built on the `Dialog` primitive and `@navi/ui` Buttons — no
 * dependency on the host app's global button classes.
 */
export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [options, setOptions] = useState<ConfirmOptions | null>(null);
  const resolverRef = useRef<((value: boolean) => void) | null>(null);

  const confirm = useCallback((opts: ConfirmOptions) => {
    setOptions(opts);
    return new Promise<boolean>((resolve) => {
      resolverRef.current = resolve;
    });
  }, []);

  const settle = useCallback((value: boolean) => {
    resolverRef.current?.(value);
    resolverRef.current = null;
    setOptions(null);
  }, []);

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      <Dialog
        isOpen={options !== null}
        onOpenChange={(open) => {
          if (!open) settle(false);
        }}
        role="alertdialog"
        title={options?.title}
        footer={
          options && (
            <>
              <Button variant="secondary" onPress={() => settle(false)} autoFocus>
                {options.cancelLabel ?? 'Cancel'}
              </Button>
              <Button variant={options.danger ? 'danger' : 'primary'} onPress={() => settle(true)}>
                {options.confirmLabel ?? 'Confirm'}
              </Button>
            </>
          )
        }
      >
        {options?.description}
      </Dialog>
    </ConfirmContext.Provider>
  );
}
