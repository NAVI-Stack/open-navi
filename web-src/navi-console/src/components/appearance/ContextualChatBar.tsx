import { useState, useRef, useMemo } from 'react';
import { useChat } from '@ai-sdk/react';
import { Button } from 'react-aria-components';
import { Bot, ChevronDown, ChevronUp, Loader2, Send } from 'lucide-react';
import { createChat } from '@/api/chats';
import { NaviChatTransport } from '@/lib/naviChatTransport';
import type { ConsoleAppearanceState } from '@/appearance/theme';
import { ContextualChatMessageRenderer } from './ContextualChatMessageRenderer';
import { useLiveEvents } from '@/hooks/useLiveEvents';
import styles from './ContextualChatBar.module.css';

interface ContextualChatBarProps {
  currentAppearance: ConsoleAppearanceState;
  onPreviewPatch: (previewTheme: ConsoleAppearanceState | null) => void;
  onApplyPatch: (patch: ConsoleAppearanceState) => void;
}

export function ContextualChatBar({
  currentAppearance,
  onPreviewPatch,
  onApplyPatch,
}: ContextualChatBarProps) {
  const [chatId, setChatId] = useState<string | null>(null);
  const [isExpanded, setIsExpanded] = useState(false);
  const [isInitializing, setIsInitializing] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const getSystemContext = () => {
    return `You are NAVI's theme appearance assistant.
Your goal is to help the user modify their appearance settings based on their instructions.

CURRENT STATE:
${JSON.stringify(currentAppearance, null, 2)}

INSTRUCTIONS:
- You may only propose changes using the valid theme-proposal schema.
- Do NOT output arbitrary CSS.
- Output exactly one fenced JSON block with your proposal.
- You must include a rationale (max 500 chars).
- The user will validate and apply the proposal; you cannot apply it directly.

Allowed Proposal Schema (Zod):
z.object({
  type: z.literal("theme-proposal"),
  version: z.literal(1),
  rationale: z.string().max(500),
  patch: z.object({
    mode: z.enum(['light', 'dark', 'system']).optional(),
    accent: z.string().optional(),
    density: z.enum(['compact', 'comfortable']).optional(),
    font_scale: z.enum(['small', 'default', 'large']).optional(),
    tokens: z.object({
      light: z.record(z.string()).optional(),
      dark: z.record(z.string()).optional()
    }).optional()
  })
})`;
  };

  const transport = useMemo(() => {
    if (!chatId) return undefined;
    return new NaviChatTransport(chatId, (_handler) => {
      // Mock subscribe for now since we rely on polling/events
      return () => {};
    }, getSystemContext);
  }, [chatId]);

  const {
    messages,
    status,
    sendMessage,
  } = useChat({
    id: chatId ?? 'temp',
    transport,
  });

  const [input, setInput] = useState('');

  useLiveEvents({
    chatId: chatId ?? undefined,
    enabled: Boolean(chatId),
  });

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!input.trim()) return;

    const submittedInput = input;
    setInput('');

    if (!chatId) {
      setIsInitializing(true);
      try {
        const chat = await createChat(undefined, "Theme Editor Chat");
        setChatId(chat.id ?? '');
        setIsExpanded(true);
        // Let state settle, then send
        setTimeout(() => {
          void sendMessage({ text: submittedInput });
        }, 100);
      } finally {
        setIsInitializing(false);
      }
    } else {
      setIsExpanded(true);
      void sendMessage({ text: submittedInput });
    }
  };

  return (
    <div className={`${styles.chatBarContainer} ${isExpanded ? styles.expanded : ''}`}>
      <div className={styles.header} onClick={() => setIsExpanded(!isExpanded)}>
        <div className={styles.title}>
          <Bot size={16} />
          <span>Appearance Assistant</span>
        </div>
        <button className={styles.toggleBtn}>
          {isExpanded ? <ChevronDown size={16} /> : <ChevronUp size={16} />}
        </button>
      </div>

      {isExpanded && (
        <div className={styles.messageList}>
          {messages.length === 0 && (
            <div className={styles.emptyState}>
              Ask me to modify the theme. For example, "Make it a dark blue theme."
            </div>
          )}
          {messages.map((msg) => (
            <ContextualChatMessageRenderer
              key={msg.id}
              message={msg}
              currentAppearance={currentAppearance}
              onPreviewPatch={onPreviewPatch}
              onApplyPatch={onApplyPatch}
            />
          ))}
          {status === 'submitted' && (
            <div className={styles.loadingState}>
              <Loader2 size={16} className={styles.spinner} />
              <span>Thinking...</span>
            </div>
          )}
        </div>
      )}

      <form onSubmit={onSubmit} className={styles.inputForm}>
        <input
          ref={inputRef}
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="Instruct Navi to modify appearance (e.g. 'Make a rose-colored theme')..."
          className={styles.inputField}
          disabled={isInitializing || status === 'submitted'}
        />
        <Button
          type="submit"
          className={styles.sendBtn}
          isDisabled={!input.trim() || isInitializing || status === 'submitted'}
        >
          {isInitializing ? <Loader2 size={16} className={styles.spinner} /> : <Send size={16} />}
        </Button>
      </form>
    </div>
  );
}
