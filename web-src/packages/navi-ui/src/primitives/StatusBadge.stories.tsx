import { StatusBadge, type StatusVariant } from './StatusBadge';
import { Stack } from './Stack';

export default { title: 'Primitives / StatusBadge' };

const ALL: StatusVariant[] = [
  'idle',
  'running',
  'blocked',
  'completed',
  'failed',
  'warning',
  'info',
  'success',
  'danger',
  'muted',
  'accent',
  'default',
];

export const AllVariants = () => (
  <Stack dir="row" gap="sm" wrap>
    {ALL.map((v) => (
      <StatusBadge key={v} variant={v} label={v} />
    ))}
  </Stack>
);
