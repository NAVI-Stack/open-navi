import { Card } from './Card';
import { Stack } from './Stack';
import { Text } from './Text';
import { Button } from './Button';

export default { title: 'Primitives / Card' };

export const Basic = () => (
  <Card>
    <Stack gap="sm">
      <Text size="lg" weight="semibold" block>
        Project Apollo
      </Text>
      <Text tone="secondary">A titled surface for grouping related content.</Text>
    </Stack>
  </Card>
);

export const WithAction = () => (
  <Card>
    <Stack gap="md">
      <Text weight="semibold" block>
        Ready to run
      </Text>
      <Text tone="secondary">NAVI will execute this directive in ACT mode.</Text>
      <Stack dir="row" gap="sm">
        <Button variant="primary">Run</Button>
        <Button variant="ghost">Cancel</Button>
      </Stack>
    </Stack>
  </Card>
);
