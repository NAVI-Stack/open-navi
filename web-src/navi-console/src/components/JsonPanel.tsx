import { useState } from 'react';
import { ChevronDown, ChevronRight, Copy, Check } from 'lucide-react';
import styles from './JsonPanel.module.css';

interface JsonPanelProps {
  data: unknown;
  label?: string;
  defaultExpanded?: boolean;
}

export function JsonPanel({ data, label, defaultExpanded = false }: JsonPanelProps) {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const [copied, setCopied] = useState(false);

  let json = '';
  try {
    json = JSON.stringify(data, null, 2);
  } catch (e) {
    json = `[Serialization Error] ${e instanceof Error ? e.message : 'Circular reference or too large'}`;
  }

  const handleCopy = () => {
    navigator.clipboard.writeText(json).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  };

  return (
    <div className={styles.panel}>
      <button
        className={styles.toggle}
        onClick={() => setExpanded(!expanded)}
        aria-expanded={expanded}
      >
        {expanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
        <span>{label ?? 'Raw JSON'}</span>
      </button>
      {expanded && (
        <div className={styles.content}>
          <button className={styles.copy} onClick={handleCopy} title="Copy JSON">
            {copied ? <Check size={12} /> : <Copy size={12} />}
          </button>
          <pre className={styles.pre}>{json}</pre>
        </div>
      )}
    </div>
  );
}
