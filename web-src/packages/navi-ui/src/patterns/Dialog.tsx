import type { ReactNode } from 'react';
import { Modal, ModalOverlay, Dialog as AriaDialog, Heading } from 'react-aria-components';
import clsx from 'clsx';
import styles from './Dialog.module.css';

export interface DialogProps {
  /** Whether the dialog is shown. */
  isOpen: boolean;
  /** Called when the open state changes (overlay click / Esc / dismiss). */
  onOpenChange: (isOpen: boolean) => void;
  /** Optional heading rendered with the dialog `title` slot. */
  title?: ReactNode;
  /** Dialog body content. */
  children?: ReactNode;
  /** Footer content, typically action buttons (right-aligned). */
  footer?: ReactNode;
  /** Allow dismissing by clicking the overlay or pressing Esc. Default true. */
  isDismissable?: boolean;
  /** `alertdialog` for confirmations that need an explicit choice. */
  role?: 'dialog' | 'alertdialog';
  /** Extra class on the dialog surface. */
  className?: string;
  /** Accessible label when no visible `title` is provided. */
  'aria-label'?: string;
}

/**
 * Token-styled, accessible modal dialog built on `react-aria-components`. The
 * foundation for confirmation dialogs and other overlay patterns — focus
 * trapping, scroll locking, and Esc/overlay dismissal come from react-aria.
 */
export function Dialog({
  isOpen,
  onOpenChange,
  title,
  children,
  footer,
  isDismissable = true,
  role = 'dialog',
  className,
  'aria-label': ariaLabel,
}: DialogProps) {
  return (
    <ModalOverlay
      className={styles.overlay}
      isOpen={isOpen}
      isDismissable={isDismissable}
      onOpenChange={onOpenChange}
    >
      <Modal className={styles.modal}>
        <AriaDialog className={clsx(styles.dialog, className)} role={role} aria-label={ariaLabel}>
          {title != null && (
            <Heading slot="title" className={styles.title}>
              {title}
            </Heading>
          )}
          {children != null && <div className={styles.body}>{children}</div>}
          {footer != null && <div className={styles.footer}>{footer}</div>}
        </AriaDialog>
      </Modal>
    </ModalOverlay>
  );
}
