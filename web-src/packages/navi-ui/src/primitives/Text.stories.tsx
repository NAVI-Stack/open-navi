import { Text } from './Text';
import { Stack } from './Stack';

export default { title: 'Primitives / Text' };

export const Sizes = () => (
  <Stack gap="sm">
    {(['xs', 'sm', 'md', 'lg', 'xl', '2xl'] as const).map((s) => (
      <Text key={s} size={s} block>
        Size {s} — the quick brown fox
      </Text>
    ))}
  </Stack>
);

export const Tones = () => (
  <Stack gap="sm">
    {(['default', 'secondary', 'tertiary', 'accent', 'danger', 'success', 'warning'] as const).map((t) => (
      <Text key={t} tone={t} block>
        Tone {t}
      </Text>
    ))}
  </Stack>
);

export const Weights = () => (
  <Stack gap="sm">
    {(['normal', 'medium', 'semibold', 'bold'] as const).map((w) => (
      <Text key={w} weight={w} block>
        Weight {w}
      </Text>
    ))}
  </Stack>
);
