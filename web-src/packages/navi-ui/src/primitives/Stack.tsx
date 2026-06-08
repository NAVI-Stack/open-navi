import type { CSSProperties, ReactNode } from 'react';
import clsx from 'clsx';
import styles from './Stack.module.css';

type Gap = 'none' | 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl';
type Align = 'start' | 'center' | 'end' | 'stretch' | 'baseline';
type Justify = 'start' | 'center' | 'end' | 'between' | 'around';

export interface StackProps {
  dir?: 'row' | 'col';
  gap?: Gap;
  align?: Align;
  justify?: Justify;
  wrap?: boolean;
  className?: string;
  style?: CSSProperties;
  children?: ReactNode;
}

/** Flexbox layout primitive driven entirely by spacing tokens. */
export function Stack({ dir = 'col', gap = 'md', align, justify, wrap, className, style, children }: StackProps) {
  return (
    <div
      className={clsx(styles.stack, className)}
      data-dir={dir}
      data-gap={gap}
      data-align={align}
      data-justify={justify}
      data-wrap={wrap ? 'true' : undefined}
      style={style}
    >
      {children}
    </div>
  );
}
