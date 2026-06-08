import React, { useState, useRef, useEffect } from 'react';
import { Button } from 'react-aria-components';
import { Loader2, Send, Plus, Mic } from 'lucide-react';
import { ModelBrowserTrigger } from '@/components/model-browser/ModelBrowserTrigger';
import { ModelBrowser } from '@/components/model-browser/ModelBrowser';
import { useModelBrowserData } from '@/components/model-browser/useModelBrowserData';
import { useToast } from '@/components/ui/Toast';
import clsx from 'clsx';
import styles from './ChatInput.module.css';

interface ChatInputProps {
  value: string;
  onChange: (value: string) => void;
  onSend: () => void;
  disabled?: boolean;
  placeholder?: string;
  isWorking?: boolean;
  status?: 'ready' | 'submitted' | 'streaming';
  onStop?: () => void;
  placement?: 'top' | 'bottom' | 'auto';
  className?: string;
}

export function ChatInput({
  value,
  onChange,
  onSend,
  disabled = false,
  placeholder = 'Message NAVI',
  status = 'ready',
  onStop,
  placement: explicitPlacement = 'auto',
  className,
}: ChatInputProps) {
  const toast = useToast();
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const wrapperRef = useRef<HTMLDivElement>(null);
  const [modelBrowserOpen, setModelBrowserOpen] = useState(false);
  const [detectedPlacement, setDetectedPlacement] = useState<'top' | 'bottom'>('top');
  const modelBrowserData = useModelBrowserData();
  const selectedModel = modelBrowserData.models.find(
    (m) => m.id === modelBrowserData.selectedId,
  ) ?? modelBrowserData.models[0];

  // Measure placement
  useEffect(() => {
    if (explicitPlacement !== 'auto') {
      setDetectedPlacement(explicitPlacement);
      return;
    }

    if (modelBrowserOpen && wrapperRef.current) {
      const rect = wrapperRef.current.getBoundingClientRect();
      const spaceAbove = rect.top;
      const spaceBelow = window.innerHeight - rect.bottom;
      if (spaceBelow > spaceAbove && spaceAbove < 550) {
        setDetectedPlacement('bottom');
      } else {
        setDetectedPlacement('top');
      }
    }
  }, [modelBrowserOpen, explicitPlacement]);

  // Auto-expand textarea
  useEffect(() => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = '0px';
    el.style.height = `${Math.min(el.scrollHeight, 220)}px`;
  }, [value]);

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      onSend();
    }
  };

  const isStreaming = status === 'streaming';
  const isSubmitted = status === 'submitted';

  return (
    <div ref={wrapperRef} className={clsx(styles.composerWrapper, className)}>
      {modelBrowserOpen && (
        <ModelBrowser
          models={modelBrowserData.models}
          providers={modelBrowserData.providers}
          selectedId={modelBrowserData.selectedId}
          favoriteIds={modelBrowserData.favoriteIds}
          isLoading={modelBrowserData.isLoading}
          onSelect={(id) => {
            void modelBrowserData.selectModel(id);
            setModelBrowserOpen(false);
          }}
          onToggleFavorite={modelBrowserData.toggleFavorite}
          onClose={() => setModelBrowserOpen(false)}
          placement={detectedPlacement}
        />
      )}
      <div className={styles.composerBox}>
        <Button
          className={styles.composerPlusBtn}
          type="button"
          onPress={() => toast.toast('File attachment not yet implemented', { variant: 'info' })}
          aria-label="Add content"
        >
          <Plus size={18} />
        </Button>

        <textarea
          ref={textareaRef}
          className={styles.composerInput}
          placeholder={isSubmitted ? 'Waiting for NAVI to respond…' : placeholder}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={handleKeyDown}
          disabled={disabled || isSubmitted}
          rows={1}
          aria-label={placeholder}
        />

        <div className={styles.composerActions}>
          <ModelBrowserTrigger
            model={selectedModel}
            isOpen={modelBrowserOpen}
            onClick={() => setModelBrowserOpen((v) => !v)}
            inline
          />

          <Button
            type="button"
            className={styles.composerMicBtn}
            isDisabled
            aria-label="Voice input unavailable"
          >
            <Mic size={18} />
          </Button>

          <Button
            className={styles.sendButton}
            onPress={isStreaming && onStop ? onStop : onSend}
            isDisabled={isStreaming ? false : !value.trim() || isSubmitted || disabled}
            aria-label={isStreaming ? 'Stop generating' : 'Send message'}
          >
            {isSubmitted ? (
              <Loader2 className={styles.spinner} size={18} />
            ) : (
              <Send size={18} />
            )}
          </Button>
        </div>
      </div>
    </div>
  );
}
