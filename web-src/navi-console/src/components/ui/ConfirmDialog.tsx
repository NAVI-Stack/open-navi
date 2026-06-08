// ConfirmDialog now lives in the shared @navi/ui library (patterns). Re-exported
// here so existing `@/components/ui/ConfirmDialog` import sites keep working
// unchanged.
export { ConfirmProvider, useConfirm } from '@navi/ui';
export type { ConfirmOptions } from '@navi/ui';
