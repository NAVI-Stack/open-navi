import { TextField, Label, Input, Text as AriaText, FieldError } from 'react-aria-components';
import clsx from 'clsx';
import styles from './Field.module.css';

export interface FieldProps {
  label?: string;
  description?: string;
  errorMessage?: string;
  placeholder?: string;
  name?: string;
  type?: 'text' | 'email' | 'password' | 'number' | 'url' | 'tel' | 'search';
  value?: string;
  defaultValue?: string;
  isRequired?: boolean;
  isDisabled?: boolean;
  onChange?: (value: string) => void;
  className?: string;
}

/** Labeled text input with description + error slots, built on react-aria. */
export function Field({
  label,
  description,
  errorMessage,
  placeholder,
  name,
  type = 'text',
  value,
  defaultValue,
  isRequired,
  isDisabled,
  onChange,
  className,
}: FieldProps) {
  return (
    <TextField
      className={clsx(styles.field, className)}
      name={name}
      type={type}
      value={value}
      defaultValue={defaultValue}
      isRequired={isRequired}
      isDisabled={isDisabled}
      isInvalid={errorMessage ? true : undefined}
      onChange={onChange}
    >
      {label && <Label className={styles.label}>{label}</Label>}
      <Input className={styles.input} placeholder={placeholder} />
      {description && !errorMessage && (
        <AriaText slot="description" className={styles.description}>
          {description}
        </AriaText>
      )}
      {errorMessage && <FieldError className={styles.error}>{errorMessage}</FieldError>}
    </TextField>
  );
}
