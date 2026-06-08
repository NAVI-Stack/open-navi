import { Button as AriaButton, type ButtonProps as AriaButtonProps } from 'react-aria-components';
import clsx from 'clsx';
import styles from './Button.module.css';

export interface ButtonProps extends Omit<AriaButtonProps, 'className' | 'style'> {
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger';
  size?: 'sm' | 'md';
  className?: string;
}

/** Token-themed, accessible button built on react-aria. */
export function Button({ variant = 'secondary', size = 'md', className, ...rest }: ButtonProps) {
  return <AriaButton {...rest} className={clsx(styles.button, styles[variant], styles[size], className)} />;
}
