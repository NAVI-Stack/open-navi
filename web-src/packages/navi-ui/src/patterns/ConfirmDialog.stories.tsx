import { useState } from 'react';
import { ConfirmProvider, useConfirm } from './ConfirmDialog';
import { Button } from '../primitives/Button';
import { Stack } from '../primitives/Stack';
import { Text } from '../primitives/Text';

export default { title: 'Patterns / ConfirmDialog' };

function Demo() {
  const confirm = useConfirm();
  const [last, setLast] = useState<string>('—');
  return (
    <Stack gap="md" align="start">
      <Stack dir="row" gap="md">
        <Button
          variant="secondary"
          onPress={async () => setLast((await confirm({ title: 'Leave page?', description: 'Unsaved changes will be lost.' })) ? 'confirmed' : 'cancelled')}
        >
          Confirm
        </Button>
        <Button
          variant="danger"
          onPress={async () =>
            setLast(
              (await confirm({ title: 'Delete directive?', description: 'This cannot be undone.', confirmLabel: 'Delete', danger: true }))
                ? 'deleted'
                : 'cancelled',
            )
          }
        >
          Destructive confirm
        </Button>
      </Stack>
      <Text tone="secondary">Last result: {last}</Text>
    </Stack>
  );
}

export const Playground = () => (
  <ConfirmProvider>
    <Demo />
  </ConfirmProvider>
);
