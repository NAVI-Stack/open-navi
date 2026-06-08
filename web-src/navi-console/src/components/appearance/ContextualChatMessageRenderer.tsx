import { useMemo, useState } from 'react';
import type { UIMessage } from 'ai';
import { Bot, Check, Play, Undo2 } from 'lucide-react';
import { Button } from 'react-aria-components';
import { ThemeProposalSchema, type ThemeProposal } from '@/appearance/schema';
import {
  type ConsoleAppearanceState,
  normalizeTheme,
  cloneAppearance,
} from '@/appearance/theme';
import { useSaveConsoleAppearance } from '@/api/appearance';
import styles from './ContextualChatMessageRenderer.module.css';

interface ContextualChatMessageRendererProps {
  message: UIMessage;
  currentAppearance: ConsoleAppearanceState;
  onPreviewPatch: (previewTheme: ConsoleAppearanceState | null) => void;
  onApplyPatch: (patch: ConsoleAppearanceState) => void;
}

export function ContextualChatMessageRenderer({
  message,
  currentAppearance,
  onPreviewPatch,
  onApplyPatch,
}: ContextualChatMessageRendererProps) {
  const saveAppearance = useSaveConsoleAppearance();
  const [isPreviewing, setIsPreviewing] = useState(false);

  // Parse text and fenced JSON
  const parts = useMemo(() => {
    let rawText = '';
    if (message.parts && Array.isArray(message.parts)) {
      rawText = message.parts
        .filter((p: any) => p.type === 'text')
        .map((p: any) => p.text)
        .join('');
    }

    const jsonMatch = rawText.match(/```json\n([\s\S]*?)\n```/);
    let proposal: ThemeProposal | null = null;
    let text = rawText;

    if (jsonMatch) {
      try {
        const parsed = JSON.parse(jsonMatch[1]);
        const validated = ThemeProposalSchema.safeParse(parsed);
        if (validated.success) {
          proposal = validated.data;
          text = rawText.replace(jsonMatch[0], '').trim();
        }
      } catch (e) {
        // invalid JSON
      }
    }
    return { text, proposal };
  }, [message.parts]);

  const { text, proposal } = parts;

  const handlePreview = () => {
    if (!proposal) return;
    const previewState = cloneAppearance(currentAppearance);
    const patchedTheme = normalizeTheme({
      ...previewState.theme,
      ...proposal.patch,
      tokens: {
        light: { ...previewState.theme.tokens.light, ...(proposal.patch.tokens?.light || {}) },
        dark: { ...previewState.theme.tokens.dark, ...(proposal.patch.tokens?.dark || {}) },
      },
    });
    previewState.theme = patchedTheme;
    setIsPreviewing(true);
    onPreviewPatch(previewState);
  };

  const handleRevert = () => {
    setIsPreviewing(false);
    onPreviewPatch(null);
  };

  const handleApply = async () => {
    if (!proposal) return;
    const previewState = cloneAppearance(currentAppearance);
    const patchedTheme = normalizeTheme({
      ...previewState.theme,
      ...proposal.patch,
      tokens: {
        light: { ...previewState.theme.tokens.light, ...(proposal.patch.tokens?.light || {}) },
        dark: { ...previewState.theme.tokens.dark, ...(proposal.patch.tokens?.dark || {}) },
      },
    });
    previewState.theme = patchedTheme;
    onApplyPatch(previewState);
    await saveAppearance.mutateAsync(previewState);
    setIsPreviewing(false);
    onPreviewPatch(null);
  };

  if (message.role === 'user') {
    return (
      <div className={styles.userMessage}>
        <span className={styles.userText}>{text}</span>
      </div>
    );
  }

  return (
    <div className={styles.assistantMessage}>
      <div className={styles.assistantHeader}>
        <Bot size={16} />
        <span>NAVI</span>
      </div>
      {text && <div className={styles.assistantText}>{text}</div>}
      {proposal && (
        <div className={styles.proposalCard}>
          <div className={styles.proposalHeader}>Theme Proposal</div>
          <div className={styles.proposalRationale}>{proposal.rationale}</div>
          <div className={styles.proposalActions}>
            {!isPreviewing ? (
              <Button onPress={handlePreview} className={styles.previewBtn}>
                <Play size={14} /> Preview
              </Button>
            ) : (
              <Button onPress={handleRevert} className={styles.revertBtn}>
                <Undo2 size={14} /> Revert
              </Button>
            )}
            <Button
              onPress={handleApply}
              className={styles.applyBtn}
              isDisabled={saveAppearance.isPending}
            >
              <Check size={14} /> {saveAppearance.isPending ? 'Saving...' : 'Apply'}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
