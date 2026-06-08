type BadgeVariant =
  | 'idle' | 'running' | 'blocked' | 'completed' | 'failed'
  | 'warning' | 'info' | 'default' | 'danger' | 'success' | 'muted' | 'accent';

// statusVariant maps a capability status string to a StatusBadge variant.
export function statusVariant(value: string | undefined): BadgeVariant {
  switch ((value ?? '').toLowerCase()) {
    case 'enabled':
    case 'available':
    case 'valid':
    case 'healthy':
    case 'configured':
    case 'running':
      return 'success';
    case 'gated':
    case 'warning':
    case 'degraded':
    case 'unconfigured':
    case 'expired':
    case 'unvalidated':
      return 'warning';
    case 'disabled':
    case 'blocked':
    case 'invalid':
    case 'failing':
    case 'error':
    case 'unavailable':
      return 'danger';
    case 'not_required':
    case 'not_applicable':
    case 'none':
    case 'unknown':
      return 'muted';
    default:
      return 'default';
  }
}
