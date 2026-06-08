import { Button } from './Button';
import { Stack } from './Stack';

export default { title: 'Primitives / Button' };

export const Variants = () => (
  <Stack dir="row" gap="md" wrap>
    <Button variant="primary">Primary</Button>
    <Button variant="secondary">Secondary</Button>
    <Button variant="ghost">Ghost</Button>
    <Button variant="danger">Danger</Button>
  </Stack>
);

export const Sizes = () => (
  <Stack dir="row" gap="md" align="center">
    <Button size="sm" variant="primary">
      Small
    </Button>
    <Button size="md" variant="primary">
      Medium
    </Button>
  </Stack>
);

export const Disabled = () => (
  <Button variant="primary" isDisabled>
    Disabled
  </Button>
);
