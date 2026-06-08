import { Inbox } from 'lucide-react';
import { EmptyState } from './EmptyState';

export default { title: 'Primitives / EmptyState' };

export const Basic = () => (
  <EmptyState title="No conversations yet" description="Start a new chat to see it appear here." />
);

export const WithIconAndAction = () => (
  <EmptyState
    icon={<Inbox size={24} aria-hidden />}
    title="Your inbox is empty"
    description="When NAVI needs your input, requests will show up here."
    action={{ label: 'Create directive', onPress: () => {} }}
  />
);
