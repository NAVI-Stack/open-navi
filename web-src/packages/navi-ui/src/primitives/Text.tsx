import type { ReactNode } from 'react';
import clsx from 'clsx';
import styles from './Text.module.css';

export interface TextProps {
  size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl';
  tone?: 'default' | 'secondary' | 'tertiary' | 'accent' | 'danger' | 'success' | 'warning';
  weight?: 'normal' | 'medium' | 'semibold' | 'bold';
  mono?: boolean;
  /** Render as a block-level <div> instead of an inline <span>. */
  block?: boolean;
  className?: string;
  children?: ReactNode;
}

/** Typography primitive mapping intent to tokens (size/tone/weight/mono). */
export function Text({ size = 'md', tone = 'default', weight = 'normal', mono, block, className, children }: TextProps) {
  const props = {
    className: clsx(styles.text, className),
    'data-size': size,
    'data-tone': tone,
    'data-weight': weight,
    'data-mono': mono ? 'true' : undefined,
  };
  return block ? <div {...props}>{children}</div> : <span {...props}>{children}</span>;
}
