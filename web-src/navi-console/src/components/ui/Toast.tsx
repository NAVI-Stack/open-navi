// Toast now lives in the shared @navi/ui library (patterns). Re-exported here so
// existing `@/components/ui/Toast` import sites keep working unchanged.
export { ToastProvider, useToast } from '@navi/ui';
export type { ToastVariant, ToastOptions, ToastAction, ToastContextValue } from '@navi/ui';
