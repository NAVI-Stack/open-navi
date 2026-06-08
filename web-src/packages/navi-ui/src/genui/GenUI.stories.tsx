import { useEffect, useRef, useState } from 'react';
import { GenUI } from './GenUI';
import { naviSpecEngine } from './NaviSpecEngine';

export default { title: 'GenUI / Renderer' };

const SPEC = JSON.stringify(
  {
    t: 'card',
    title: 'Create project',
    children: [
      { t: 'text', value: 'NAVI rendered this from a JSON spec.', tone: 'secondary' },
      { t: 'divider' },
      {
        t: 'form',
        action: { intent: 'create_project' },
        submitLabel: 'Create',
        children: [{ t: 'field', name: 'title', label: 'Project name', placeholder: 'e.g. Apollo', required: true }],
      },
      {
        t: 'stack',
        dir: 'row',
        gap: 'sm',
        children: [
          { t: 'badge', label: 'ACT', status: 'running' },
          { t: 'badge', label: 'draft', status: 'idle' },
        ],
      },
    ],
  },
  null,
  2,
);

export const NativeSpec = () => <GenUI source={SPEC} onAction={() => {}} />;

export const InvalidSpecShowsError = () => <GenUI source={{ t: 'totally-unknown' }} />;

export const Streaming = () => {
  const [pos, setPos] = useState(0);
  const timer = useRef<ReturnType<typeof setInterval> | null>(null);
  useEffect(() => {
    timer.current = setInterval(() => {
      setPos((p) => {
        if (p >= SPEC.length) {
          if (timer.current) clearInterval(timer.current);
          return p;
        }
        return p + 4;
      });
    }, 50);
    return () => {
      if (timer.current) clearInterval(timer.current);
    };
  }, []);
  return <GenUI source={SPEC.slice(0, pos)} streaming />;
};

export const SystemPrompt = () => (
  <pre style={{ whiteSpace: 'pre-wrap', fontSize: 12, color: 'var(--navi-text-secondary)' }}>
    {naviSpecEngine.systemPrompt()}
  </pre>
);
