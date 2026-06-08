import clsx from 'clsx';
import { ChevronDown, Cloud, HardDrive, HelpCircle, Sparkles } from 'lucide-react';
import type { BrowserModel } from './types';
import styles from './ModelBrowserTrigger.module.css';

interface ModelBrowserTriggerProps {
  model: BrowserModel | undefined;
  isOpen: boolean;
  onClick: () => void;
  inline?: boolean;
}

export function ModelBrowserTrigger({ model, isOpen, onClick, inline = false }: ModelBrowserTriggerProps) {
  const displayName = model?.displayName ?? 'Select model';

  const Icon =
    model?.executionType === 'auto' ? Sparkles :
    model?.executionType === 'local' ? HardDrive :
    model?.executionType === 'cloud' ? Cloud :
    HelpCircle;

  const triggerButton = (
    <button
      type="button"
      className={clsx(
        styles.trigger,
        inline && styles.inlineTrigger,
        isOpen && styles.triggerOpen
      )}
      onClick={onClick}
      aria-haspopup="listbox"
      aria-expanded={isOpen}
      aria-label={`Model selector — currently ${displayName}`}
    >
      <Icon size={13} className={styles.triggerIcon} aria-hidden />
      <span className={styles.triggerName}>{displayName}</span>
      <ChevronDown
        size={12}
        className={clsx(styles.chevron, isOpen && styles.chevronOpen)}
        aria-hidden
      />
    </button>
  );

  if (inline) {
    return triggerButton;
  }

  return (
    <div className={styles.triggerRow}>
      {triggerButton}
    </div>
  );
}
