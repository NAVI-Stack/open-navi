import { useState } from 'react';
import { Dialog } from './Dialog';
import { Button } from '../primitives/Button';
import { Text } from '../primitives/Text';

export default { title: 'Patterns / Dialog' };

export const Controlled = () => {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="primary" onPress={() => setOpen(true)}>
        Open dialog
      </Button>
      <Dialog
        isOpen={open}
        onOpenChange={setOpen}
        title="Share workspace"
        footer={
          <>
            <Button variant="secondary" onPress={() => setOpen(false)}>
              Cancel
            </Button>
            <Button variant="primary" onPress={() => setOpen(false)}>
              Share
            </Button>
          </>
        }
      >
        <Text tone="secondary">Anyone with the link will be able to view this workspace.</Text>
      </Dialog>
    </>
  );
};
