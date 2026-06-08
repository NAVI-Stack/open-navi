import { useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeHighlight from 'rehype-highlight';
import { Copy, Check } from 'lucide-react';
import 'highlight.js/styles/github-dark.css';
import styles from './MarkdownRenderer.module.css';

function extractText(node: unknown): string {
  if (!node || typeof node !== 'object') return '';
  const n = node as { value?: string; children?: unknown[] };
  if (typeof n.value === 'string') return n.value;
  if (!Array.isArray(n.children)) return '';
  return n.children.map(extractText).join('');
}

function CodeBlock({ code, language }: { code: string; language?: string }) {
  const [copied, setCopied] = useState(false);
  const handleCopy = () => {
    navigator.clipboard.writeText(code).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  };
  return (
    <div className={styles.codeBlock}>
      <div className={styles.codeHeader}>
        <span className={styles.codeLang}>{language || 'code'}</span>
        <button className={styles.copyBtn} onClick={handleCopy} type="button" aria-label="Copy code">
          {copied ? <Check size={12} /> : <Copy size={12} />}
          {copied ? 'Copied' : 'Copy'}
        </button>
      </div>
      <pre className={styles.pre}>
        <code>{code}</code>
      </pre>
    </div>
  );
}

interface Props {
  content: string;
  className?: string;
}

// MarkdownRenderer renders assistant/user message bodies with GFM + syntax
// highlighting, styled with the console's --navi-* tokens. Raw HTML is NOT
// enabled (no rehype-raw) to avoid XSS from model/user content.
export function MarkdownRenderer({ content, className }: Props) {
  return (
    <div className={className ? `${styles.prose} ${className}` : styles.prose}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeHighlight]}
        components={{
          h1: ({ children }) => <h1 className={styles.h1}>{children}</h1>,
          h2: ({ children }) => <h2 className={styles.h2}>{children}</h2>,
          h3: ({ children }) => <h3 className={styles.h3}>{children}</h3>,
          h4: ({ children }) => <h4 className={styles.h4}>{children}</h4>,
          p: ({ children }) => <p className={styles.p}>{children}</p>,
          ul: ({ children }) => <ul className={styles.ul}>{children}</ul>,
          ol: ({ children }) => <ol className={styles.ol}>{children}</ol>,
          li: ({ children }) => <li className={styles.li}>{children}</li>,
          blockquote: ({ children }) => <blockquote className={styles.blockquote}>{children}</blockquote>,
          a: ({ href, children }) => (
            <a className={styles.link} href={href} target="_blank" rel="noopener noreferrer">
              {children}
            </a>
          ),
          strong: ({ children }) => <strong className={styles.strong}>{children}</strong>,
          em: ({ children }) => <em className={styles.em}>{children}</em>,
          del: ({ children }) => <del className={styles.del}>{children}</del>,
          hr: () => <hr className={styles.hr} />,
          code: ({ className: cls, children }) => {
            if (!cls) {
              return <code className={styles.inlineCode}>{children}</code>;
            }
            return <code className={cls}>{children}</code>;
          },
          pre: ({ node, children }) => {
            const codeNode = (node as unknown as { children?: Array<Record<string, unknown>> })?.children?.[0];
            const props = (codeNode?.properties ?? {}) as { className?: string[] };
            const cls = Array.isArray(props.className) ? props.className.join(' ') : '';
            const lang = /language-(\w+)/.exec(cls)?.[1] || '';
            const raw = extractText(codeNode);
            const fallback = (children as React.ReactElement<{ children?: string }>)?.props?.children;
            const code = (raw || (typeof fallback === 'string' ? fallback : '')).replace(/\n$/, '');
            return <CodeBlock code={code || ' '} language={lang} />;
          },
          table: ({ children }) => (
            <div className={styles.tableWrap}>
              <table className={styles.table}>{children}</table>
            </div>
          ),
          th: ({ children }) => <th className={styles.th}>{children}</th>,
          td: ({ children }) => <td className={styles.td}>{children}</td>,
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
}
