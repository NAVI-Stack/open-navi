import { Field } from './Field';
import { Stack } from './Stack';

export default { title: 'Primitives / Field' };

export const Basic = () => (
  <Stack gap="lg" style={{ maxWidth: 360 }}>
    <Field name="title" label="Project name" placeholder="e.g. Apollo" />
    <Field name="goal" label="Goal" description="What should NAVI accomplish?" />
    <Field name="email" label="Email" type="email" placeholder="you@example.com" isRequired />
    <Field name="locked" label="Read only" defaultValue="can't touch this" isDisabled />
  </Stack>
);

export const WithError = () => (
  <Stack style={{ maxWidth: 360 }}>
    <Field name="port" label="Port" defaultValue="not-a-number" errorMessage="Port must be a number." />
  </Stack>
);
