import { Stack } from './Stack';
import { StatusBadge } from './StatusBadge';

export default { title: 'Primitives / Stack' };

const Box = ({ children }: { children: string }) => <StatusBadge variant="accent" label={children} />;

export const Row = () => (
  <Stack dir="row" gap="md">
    <Box>one</Box>
    <Box>two</Box>
    <Box>three</Box>
  </Stack>
);

export const Column = () => (
  <Stack dir="col" gap="sm" align="start">
    <Box>top</Box>
    <Box>middle</Box>
    <Box>bottom</Box>
  </Stack>
);

export const Wrapped = () => (
  <Stack dir="row" gap="sm" wrap style={{ maxWidth: 220 }}>
    {Array.from({ length: 10 }, (_, i) => (
      <Box key={i}>{`item ${i + 1}`}</Box>
    ))}
  </Stack>
);
