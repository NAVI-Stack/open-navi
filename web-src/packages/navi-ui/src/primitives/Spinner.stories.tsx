import { Spinner } from './Spinner';
import { Stack } from './Stack';

export default { title: 'Primitives / Spinner' };

export const Sizes = () => (
  <Stack dir="row" gap="lg" align="center">
    <Spinner size={16} />
    <Spinner size={24} />
    <Spinner size={40} />
  </Stack>
);
