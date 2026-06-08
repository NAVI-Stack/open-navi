import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { CheckCircle2, AlertTriangle, Info, X } from 'lucide-react';
import styles from './Toast.module.css';

export type ToastVariant = 'success' | 'error' | 'info';

export interface ToastAction {
  label: string;
  onPress: () => void;
}

export interface ToastOptions {
  variant?: ToastVariant;
  /** Auto-dismiss delay in ms. Defaults to 5000. Pass 0 to disable auto-dismiss. */
  duration?: number;
  action?: ToastAction;
}

interface ToastRecord extends ToastOptions {
  id: number;
  message: string;
}

export interface ToastContextValue {
  toast: (message: string, options?: ToastOptions) => number;
  success: (message: string, options?: Omit<ToastOptions, 'variant'>) => number;
  error: (message: string, options?: Omit<ToastOptions, 'variant'>) => number;
  dismiss: (id: number) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

// Fallback used when no ToastProvider is mounted (e.g. a component rendered in
// isolation under test). Toasts are non-critical UI, so we degrade to the
// console rather than crashing the component tree.
const fallbackToast: ToastContextValue = {
  toast: (message) => {
    console.info('[toast]', message);
    return 0;
  },
  success: (message) => {
    console.info('[toast:success]', message);
    return 0;
  },
  error: (message) => {
    console.error('[toast:error]', message);
    return 0;
  },
  dismiss: () => {},
};

export function useToast(): ToastContextValue {
  return useContext(ToastContext) ?? fallbackToast;
}

const ICONS: Record<ToastVariant, ReactNode> = {
  success: <CheckCircle2 size={16} aria-hidden />,
  error: <AlertTriangle size={16} aria-hidden />,
  info: <Info size={16} aria-hidden />,
};

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastRecord[]>([]);
  const idRef = useRef(0);

  const dismiss = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const toast = useCallback((message: string, options: ToastOptions = {}) => {
    const id = ++idRef.current;
    setToasts((prev) => [...prev, { id, message, variant: 'info', duration: 5000, ...options }]);
    return id;
  }, []);

  const success = useCallback(
    (message: string, options: Omit<ToastOptions, 'variant'> = {}) =>
      toast(message, { ...options, variant: 'success' }),
    [toast],
  );

  const error = useCallback(
    (message: string, options: Omit<ToastOptions, 'variant'> = {}) =>
      toast(message, { ...options, variant: 'error', duration: options.duration ?? 8000 }),
    [toast],
  );

  return (
    <ToastContext.Provider value={{ toast, success, error, dismiss }}>
      {children}
      {createPortal(
        <div className={styles.viewport} role="region" aria-label="Notifications">
          {toasts.map((t) => (
            <ToastItem key={t.id} toast={t} onDismiss={dismiss} />
          ))}
        </div>,
        document.body,
      )}
    </ToastContext.Provider>
  );
}

function ToastItem({ toast, onDismiss }: { toast: ToastRecord; onDismiss: (id: number) => void }) {
  const variant = toast.variant ?? 'info';
  useEffect(() => {
    if (!toast.duration) return;
    const timer = setTimeout(() => onDismiss(toast.id), toast.duration);
    return () => clearTimeout(timer);
  }, [toast.id, toast.duration, onDismiss]);

  return (
    <div className={styles.toast} data-variant={variant} role="status">
      <span className={styles.icon}>{ICONS[variant]}</span>
      <span className={styles.message}>{toast.message}</span>
      {toast.action && (
        <button
          type="button"
          className={styles.action}
          onClick={() => {
            toast.action!.onPress();
            onDismiss(toast.id);
          }}
        >
          {toast.action.label}
        </button>
      )}
      <button
        type="button"
        className={styles.close}
        aria-label="Dismiss notification"
        onClick={() => onDismiss(toast.id)}
      >
        <X size={14} aria-hidden />
      </button>
    </div>
  );
}
