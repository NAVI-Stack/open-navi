import { ToastProvider, useToast } from './Toast';
import { Button } from '../primitives/Button';
import { Stack } from '../primitives/Stack';

export default { title: 'Patterns / Toast' };

function Demo() {
  const { success, error, toast } = useToast();
  return (
    <Stack dir="row" gap="md" wrap>
      <Button variant="primary" onPress={() => success('Saved successfully')}>
        Success
      </Button>
      <Button variant="danger" onPress={() => error('Something went wrong')}>
        Error
      </Button>
      <Button
        variant="secondary"
        onPress={() => toast('Heads up', { action: { label: 'Undo', onPress: () => {} } })}
      >
        Info + action
      </Button>
    </Stack>
  );
}

export const Playground = () => (
  <ToastProvider>
    <Demo />
  </ToastProvider>
);
