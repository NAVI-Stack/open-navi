import { useState, useEffect, useRef } from 'react';
import styles from './CeremonyControls.module.css';

export interface CeremonyControlOption {
  key: string;
  label: string;
  description?: string;
}

export interface CeremonyControlAction {
  key: string;
  label: string;
  variant: 'primary' | 'secondary' | 'ghost';
}

export interface CeremonyControlsProps {
  step: string;
  type?: 'single_select' | 'multi_select' | 'actions';
  options?: CeremonyControlOption[];
  actions?: CeremonyControlAction[];
  defaults?: Record<string, boolean>;
  onSelect: (step: string, value: string | Record<string, boolean>) => void;
  disabled?: boolean;
}

export function CeremonyControls({
  step,
  type,
  options = [],
  actions = [],
  defaults = {},
  onSelect,
  disabled = false,
}: CeremonyControlsProps) {
  const [selected, setSelected] = useState<string | null>(null);
  const [checked, setChecked] = useState<Record<string, boolean>>(() => {
    const init: Record<string, boolean> = {};
    for (const opt of options) {
      init[opt.key] = defaults[opt.key] ?? false;
    }
    return init;
  });

  // Trigger entrance animations after a short delay so the message bubble has settled.
  const mountedRef = useRef(false);
  const [animateIn, setAnimateIn] = useState(false);
  useEffect(() => {
    if (mountedRef.current) return;
    mountedRef.current = true;
    const t = setTimeout(() => setAnimateIn(true), 120);
    return () => clearTimeout(t);
  }, []);

  if (type === 'single_select') {
    return (
      <div className={styles.chips} role="group" aria-label="Choose an option">
        {options.map((opt, i) => (
          <button
            key={opt.key}
            type="button"
            className={`${styles.chip} ${styles.chipEnter} ${selected === opt.key ? styles.chipSelected : ''}`}
            style={{ animationDelay: `${i * 55}ms` }}
            disabled={disabled}
            onClick={() => {
              setSelected(opt.key);
              onSelect(step, opt.key);
            }}
            aria-pressed={selected === opt.key}
          >
            <span className={styles.chipLabel}>{opt.label}</span>
            {opt.description && <span className={styles.chipDesc}>{opt.description}</span>}
            {selected === opt.key && <span className={styles.chipSelectedDot} aria-hidden="true" />}
          </button>
        ))}
      </div>
    );
  }

  if (type === 'multi_select') {
    // Compute stagger delays only for initially-checked items.
    let checkedIdx = 0;
    const itemDelays: number[] = options.map((opt) =>
      defaults[opt.key] ? (checkedIdx++) * 90 : 0,
    );

    return (
      <div className={styles.multiSelect}>
        <ul className={styles.checkList} role="group" aria-label="Choose actions to confirm">
          {options.map((opt, i) => {
            const isDefaultChecked = defaults[opt.key] ?? false;
            const isChecked = checked[opt.key] ?? false;
            return (
              <li key={opt.key} className={styles.checkItem}>
                <label className={styles.checkLabel}>
                  <span className={styles.checkBoxWrapper}>
                    <input
                      type="checkbox"
                      className={styles.checkBoxNative}
                      checked={isChecked}
                      disabled={disabled}
                      onChange={(e) =>
                        setChecked((prev) => ({ ...prev, [opt.key]: e.target.checked }))
                      }
                    />
                    <span
                      className={styles.checkBoxVisual}
                      data-checked={String(isChecked)}
                    >
                      {isChecked && (
                        <svg
                          className={`${styles.checkMark} ${animateIn && isDefaultChecked ? styles.checkMarkAnimateIn : ''}`}
                          style={
                            animateIn && isDefaultChecked
                              ? { animationDelay: `${itemDelays[i]}ms` }
                              : undefined
                          }
                          width="10"
                          height="8"
                          viewBox="0 0 10 8"
                          fill="none"
                          aria-hidden="true"
                        >
                          <path
                            d="M1 4L3.5 6.5L9 1"
                            stroke="currentColor"
                            strokeWidth="1.8"
                            strokeLinecap="round"
                            strokeLinejoin="round"
                          />
                        </svg>
                      )}
                    </span>
                  </span>
                  <span className={styles.checkLabelText}>{opt.label}</span>
                </label>
              </li>
            );
          })}
        </ul>
        <button
          type="button"
          className={styles.doneBtn}
          disabled={disabled}
          onClick={() => onSelect(step, checked)}
        >
          Done
        </button>
      </div>
    );
  }

  if (type === 'actions') {
    return (
      <div className={styles.actions}>
        {actions.map((action) => (
          <button
            key={action.key}
            type="button"
            className={`${styles.actionBtn} ${styles[`actionBtn_${action.variant}`]}`}
            disabled={disabled}
            onClick={() => onSelect(step, action.key)}
          >
            {action.label}
          </button>
        ))}
      </div>
    );
  }

  return null;
}
