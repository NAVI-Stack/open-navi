import {
  useMemo,
  useRef,
  useEffect,
  useState,
  useCallback,
} from 'react';
import { useChat } from '@ai-sdk/react';
import type { UIMessage } from 'ai';
import {
  AlertCircle,
  ArrowDown,
  Bot,
  Loader2,
  Sparkles,
} from 'lucide-react';
import { ChatInput } from '@/components/chat/ChatInput';
import {
  archiveChat,
  createChat,
  useChatThread,
  useChatRuntimeSummary,
  sendChatMessage,
  sendMessageFeedback,
  editAndResendMessage,
  regenerateLastReply,
  continueLastReply,
  selectMessageVariant,
} from '@/api/chats';
import { useQueryClient } from '@tanstack/react-query';
import { useNavigate, useRoute } from '@/app/router';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { useLiveEventsContext } from '@/hooks/LiveEventsContext';
import { useLiveEvents } from '@/hooks/useLiveEvents';
import { DRAFT_RESPONSE_TIMEOUT_MS, isDraftChatId } from '@/lib/newChat';
import { shouldSyncServerMessages } from '@/lib/chatMessageSync';
import { NaviChatTransport } from '@/lib/naviChatTransport';
import { listenForNaviAssistantReply } from '@/lib/naviLiveStream';
import type { ChatMessage as ChatMessageType } from '@/types/api';
import type { LiveEvent, LiveFrame } from '@/types/events';
import {
  ChatMessage,
  type ChatMessageView,
  type FeedbackRating,
  type ToolPart,
} from '@/components/chat/ChatMessage';
import type { CeremonyControlOption, CeremonyControlAction } from '@/components/chat/CeremonyControls';
import { parseRenderPayload, type ChatRenderPayload } from '@/components/chat/renderPayload';
import { TypingIndicator } from '@/components/chat/TypingIndicator';
import { useCeremonyStep } from '@/api/ceremony';
import { useRail, useLocalWorking } from '@/components/shell/AppShell';
import styles from './ChatPage.module.css';

// ─── Shared constants ─────────────────────────────────────────────────────────

interface ChatPageProps {
  chatId: string;
  projectId?: string;
}

const STARTER_PROMPTS = [
  {
    title: 'Analyze Runs',
    desc: 'Check the status of recent execution runs and errors.',
    prompt:
      'Show me the recent execution runs and tell me if there are any failures.',
  },
  {
    title: 'Workspace Audit',
    desc: 'List active tasks and workspaces in the current context.',
    prompt: 'What workspaces are active right now? List the active tasks.',
  },
  {
    title: 'Create Task Plan',
    desc: 'Decompose a goal into specific structured tasks.',
    prompt:
      'Help me draft a task decomposition plan for building a new agent capability.',
  },
  {
    title: 'System Health Check',
    desc: 'Check the current health and status of NAVI services.',
    prompt: 'Check system health and show me any active alerts or logs.',
  },
];

// ─── Entry point ──────────────────────────────────────────────────────────────

export function ChatPage({ chatId, projectId }: ChatPageProps) {
  const isDraft = isDraftChatId(chatId);
  const { query } = useRoute();
  const navigate = useNavigate();

  if (isDraft) {
    return (
      <DraftChatComposer
        freshKey={query.get('fresh')}
        composeText={query.get('compose') ?? ''}
        projectId={projectId}
        navigate={navigate}
      />
    );
  }

  return <ExistingChatView chatId={chatId} projectId={projectId} />;
}

// ─── Existing chat — AI SDK useChat ───────────────────────────────────────────

function ExistingChatView({
  chatId,
  projectId,
}: {
  chatId: string;
  projectId?: string;
}) {
  const navigate = useNavigate();
  const { setLocalWorking } = useLocalWorking();
  const live = useLiveEventsContext();
  const { data: chatData, isLoading } = useChatThread(chatId, {
    refetchInterval: 15_000,
  });
  const { data: summary } = useChatRuntimeSummary(chatId);
  const queryClient = useQueryClient();
  const scrollRef = useRef<HTMLDivElement>(null);
  const [inputText, setInputText] = useState('');
  const [sendError, setSendError] = useState<string | null>(null);
  // Tool-invocation chips for the in-flight run, accumulated from live events.
  const [liveToolParts, setLiveToolParts] = useState<ToolPart[]>([]);
  const initializedRef = useRef(false);
  const atBottomRef = useRef(true);
  const [showScrollFab, setShowScrollFab] = useState(false);

  const transport = useMemo(
    () => new NaviChatTransport(chatId, live.subscribe),
    [chatId, live.subscribe],
  );

  const serverMessages = useMemo((): UIMessage[] => {
    if (!chatData?.messages) return [];
    return chatData.messages
      .map(toUIMessage)
      .filter((m): m is UIMessage => m !== null);
  }, [chatData?.messages]);

  const isCeremonyThread = useMemo(() => {
    const metadata = (chatData as Record<string, unknown> | undefined)?.metadata as
      | Record<string, unknown>
      | undefined;
    const firstRun = metadata?.firstRunThread as Record<string, unknown> | undefined;
    return firstRun?.type === 'navi_ceremony';
  }, [chatData]);

  const {
    messages,
    sendMessage,
    status,
    error: streamError,
    setMessages,
    stop,
  } = useChat({
    id: chatId,
    transport,
    onFinish: () => {
      void refreshChatQueries(queryClient, chatId, projectId, true).then(() => {
        if (!isCeremonyThread) return;
        void queryClient.refetchQueries({ queryKey: ['chat', chatId] });
      });
    },
    onError: (err) => {
      setSendError(err.message ?? 'Something went wrong. Try again.');
    },
  });

  // Seed useChat from server on first load, then sync when polling catches new messages.
  useEffect(() => {
    if (!serverMessages.length) return;
    if (!initializedRef.current) {
      setMessages(serverMessages);
      initializedRef.current = true;
      return;
    }
    const ceremonyNeedsSync =
      isCeremonyThread &&
      (serverMessages.length > messages.length ||
        serverMessages.at(-1)?.id !== messages.at(-1)?.id);
    if (ceremonyNeedsSync || shouldSyncServerMessages(serverMessages, messages, status)) {
      setMessages(serverMessages);
    }
  }, [serverMessages, messages, status, setMessages, isCeremonyThread]);

  // Reset when chatId changes
  useEffect(() => {
    initializedRef.current = false;
    setSendError(null);
    setInputText('');
    setLiveToolParts([]);
  }, [chatId]);

  // Accumulate live tool-invocation chips for the current run. Cleared on
  // terminal events so the reseeded server thread (with persisted parts) wins.
  useEffect(() => {
    return live.subscribe((item) => {
      const event = liveEventRecord(item.frame);
      if (!event) return;
      const type = stringValue(event.type);
      const payload = recordValue(event.payload);
      if (!type || !eventBelongsToChat(event, payload, chatId)) return;
      if (type === 'tool.call.started') {
        const callId = stringValue(payload?.call_id);
        if (!callId) return;
        const toolName = stringValue(payload?.tool_name) ?? 'tool';
        setLiveToolParts((prev) =>
          prev.some((p) => p.toolInvocationId === callId)
            ? prev
            : [...prev, { toolInvocationId: callId, toolName, state: 'call' }],
        );
      } else if (type === 'tool.call.completed' || type === 'tool.call.failed') {
        const callId = stringValue(payload?.call_id);
        if (!callId) return;
        const isError = type === 'tool.call.failed';
        const result = isError ? stringValue(payload?.error) : toolResultValue(payload?.result);
        const toolName = stringValue(payload?.tool_name) ?? 'tool';
        setLiveToolParts((prev) => {
          const idx = prev.findIndex((p) => p.toolInvocationId === callId);
          if (idx === -1) {
            return [...prev, { toolInvocationId: callId, toolName, state: 'result', result, isError }];
          }
          const next = prev.slice();
          next[idx] = { ...next[idx], state: 'result', result: result ?? next[idx].result, isError };
          return next;
        });
      } else if (TERMINAL_EVENT_TYPES.has(type) || RUN_FAILURE_EVENT_TYPES.has(type)) {
        setLiveToolParts([]);
        if (
          isCeremonyThread &&
          (type === 'assistant.message.completed' || type === 'run.completed')
        ) {
          void refreshChatQueries(queryClient, chatId, projectId, false);
        }
      }
    });
  }, [live.subscribe, chatId, isCeremonyThread, projectId, queryClient]);

  const handleScroll = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    const dist = el.scrollHeight - el.scrollTop - el.clientHeight;
    atBottomRef.current = dist < 80;
    setShowScrollFab(dist > 200);
  }, []);

  const scrollToBottom = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' });
    atBottomRef.current = true;
    setShowScrollFab(false);
  }, []);

  // Assistant feedback (👍/👎) — optimistic local override on top of persisted metadata.
  const [feedbackOverrides, setFeedbackOverrides] = useState<Record<string, FeedbackRating>>({});
  const feedbackFor = useCallback(
    (id: string): FeedbackRating => {
      if (id in feedbackOverrides) return feedbackOverrides[id];
      const m = chatData?.messages?.find((x) => (x.id ?? '') === id);
      const raw = (m?.metadata as Record<string, unknown> | undefined)?.feedback;
      return raw === 'up' || raw === 'down' ? raw : null;
    },
    [feedbackOverrides, chatData?.messages],
  );
  const handleFeedback = useCallback(
    (messageId: string, rating: FeedbackRating) => {
      setFeedbackOverrides((prev) => ({ ...prev, [messageId]: rating }));
      sendMessageFeedback(chatId, messageId, rating)
        .then(() => queryClient.invalidateQueries({ queryKey: ['chat', chatId] }))
        .catch(() => {
          setFeedbackOverrides((prev) => {
            const next = { ...prev };
            delete next[messageId];
            return next;
          });
        });
    },
    [chatId, queryClient],
  );

  const handleEditResend = useCallback(
    (messageId: string, newContent: string) => {
      setSendError(null);
      editAndResendMessage(chatId, messageId, newContent)
        .then(() => {
          // Thread was truncated + re-run server-side; reseed the view from the server.
          initializedRef.current = false;
          setMessages([]);
          return refreshChatQueries(queryClient, chatId, projectId, true);
        })
        .catch((err) => setSendError(err instanceof Error ? err.message : String(err)));
    },
    [chatId, projectId, queryClient, setMessages],
  );

  const handleRegenerate = useCallback(() => {
    setSendError(null);
    regenerateLastReply(chatId)
      .then(() => {
        initializedRef.current = false;
        setMessages([]);
        return refreshChatQueries(queryClient, chatId, projectId, true);
      })
      .catch((err) => setSendError(err instanceof Error ? err.message : String(err)));
  }, [chatId, projectId, queryClient, setMessages]);

  const handleContinue = useCallback(() => {
    setSendError(null);
    continueLastReply(chatId)
      .then(() => {
        initializedRef.current = false;
        setMessages([]);
        return refreshChatQueries(queryClient, chatId, projectId, true);
      })
      .catch((err) => setSendError(err instanceof Error ? err.message : String(err)));
  }, [chatId, projectId, queryClient, setMessages]);

  const handleSelectVariant = useCallback(
    (messageId: string, index: number) => {
      setSendError(null);
      selectMessageVariant(chatId, messageId, index)
        .then(() => refreshChatQueries(queryClient, chatId, projectId, true))
        .catch((err) => setSendError(err instanceof Error ? err.message : String(err)));
    },
    [chatId, projectId, queryClient],
  );

  const handleSend = useCallback(() => {
    const text = inputText.trim();
    if (!text || status === 'submitted' || status === 'streaming') return;
    setSendError(null);
    setInputText('');
    setLiveToolParts([]);
    void sendMessage({ text });
  }, [inputText, status, sendMessage]);

  // Persisted tool chips read back from a completed message's metadata.
  const persistedToolParts = useCallback(
    (id: string): ToolPart[] | undefined => {
      const m = chatData?.messages?.find((x) => (x.id ?? '') === id);
      return parseToolParts((m?.metadata as Record<string, unknown> | undefined)?.toolParts);
    },
    [chatData?.messages],
  );

  // Auto-collapse the sidebar when entering a ceremony thread so NAVI gets full focus.
  const rail = useRail();
  useEffect(() => {
    if (isCeremonyThread && !isLoading) {
      rail.silentCollapse();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isCeremonyThread, isLoading]);

  const [ceremonyStreamActive, setCeremonyStreamActive] = useState(false);

  const ceremonyStep = useCeremonyStep();

  const ceremonyMetaFor = useCallback(
    (messageId: string): Partial<ChatMessageView> => {
      const m = chatData?.messages?.find((x) => (x.id ?? '') === messageId);
      if (!m) return {};
      const meta = m.metadata as Record<string, unknown> | undefined;
      return {
        ceremonyStep: meta?.ceremonyStep as string | undefined,
        ceremonyType: meta?.ceremonyType as string | undefined,
        ceremonyControls: meta?.ceremonyControls as CeremonyControlOption[] | undefined,
        ceremonyActions: meta?.ceremonyActions as CeremonyControlAction[] | undefined,
        ceremonyDefaults: meta?.ceremonyDefaults as Record<string, boolean> | undefined,
      };
    },
    [chatData?.messages],
  );

  const renderPayloadFor = useCallback(
    (messageId: string): ChatRenderPayload | undefined => {
      const m = chatData?.messages?.find((x) => (x.id ?? '') === messageId);
      if (!m) return undefined;
      return parseRenderPayload(m.metadata) ?? undefined;
    },
    [chatData?.messages],
  );

  const updateStreamingAssistant = useCallback(
    (assistantId: string, content: string) => {
      setMessages((prev) =>
        prev.map((m) =>
          m.id === assistantId
            ? { ...m, parts: [{ type: 'text' as const, text: content }] }
            : m,
        ),
      );
    },
    [setMessages],
  );

  const displayMessages = useMemo((): ChatMessageView[] => {
    return messages
      .map((msg) => {
        const base = uiMessageToChatView(msg);
        if (!base) return null;
        if (isCeremonyThread && base.role === 'assistant') {
          return { ...base, ...ceremonyMetaFor(base.id) };
        }
        if (base.role === 'assistant') {
          const renderPayload = renderPayloadFor(base.id);
          if (renderPayload) return { ...base, renderPayload };
        }
        return base;
      })
      .filter((m): m is ChatMessageView => m !== null);
  }, [messages, isCeremonyThread, ceremonyMetaFor, renderPayloadFor]);

  // Smart auto-scroll: only stick to the bottom when the user is already there.
  useEffect(() => {
    const el = scrollRef.current;
    if (el && atBottomRef.current) {
      el.scrollTop = el.scrollHeight;
    }
  }, [displayMessages.length, status]);

  const handleCeremonySelect = useCallback(
    (step: string, value: string | Record<string, boolean>) => {
      setSendError(null);
      const assistantId = crypto.randomUUID();
      setCeremonyStreamActive(true);
      setMessages((prev) => [
        ...prev,
        { id: assistantId, role: 'assistant', parts: [{ type: 'text', text: '' }] },
      ]);

      const stopListen = listenForNaviAssistantReply(live.subscribe, chatId, {
        onPartial: (content) => updateStreamingAssistant(assistantId, content),
        onTerminal: (content) => {
          if (content) updateStreamingAssistant(assistantId, content);
          setCeremonyStreamActive(false);
          void refreshChatQueries(queryClient, chatId, projectId, false);
          queryClient.invalidateQueries({ queryKey: ['ceremony'] });
        },
        onError: (message) => {
          setCeremonyStreamActive(false);
          setSendError(message ?? 'Could not continue setup. Try again.');
        },
      });

      ceremonyStep.mutate(
        { chatId, step, value },
        {
          onSuccess: (data) => {
            if (!data.redirect) return;
            stopListen();
            setCeremonyStreamActive(false);
            setMessages((prev) => prev.filter((m) => m.id !== assistantId));
            void refreshChatQueries(queryClient, chatId, projectId, false);
            queryClient.invalidateQueries({ queryKey: ['ceremony'] });
            navigate(data.redirect, true);
          },
          onError: (err) => {
            stopListen();
            setCeremonyStreamActive(false);
            setMessages((prev) => prev.filter((m) => m.id !== assistantId));
            setSendError(err.message ?? 'Could not continue setup. Try again.');
          },
        },
      );
    },
    [
      ceremonyStep,
      chatId,
      projectId,
      queryClient,
      live.subscribe,
      setMessages,
      updateStreamingAssistant,
      navigate,
    ],
  );

  const isWorking =
    status === 'submitted' ||
    status === 'streaming' ||
    summary?.status === 'running';

  useEffect(() => {
    setLocalWorking(isWorking);
    return () => setLocalWorking(false);
  }, [isWorking, setLocalWorking]);

  const showTyping =
    (isWorking || (isCeremonyThread && ceremonyStreamActive)) &&
    messages.at(-1)?.role !== 'assistant';

  // Id of the most recent assistant message (regenerate target).
  const lastAssistantId = (() => {
    for (let i = messages.length - 1; i >= 0; i--) {
      if (messages[i].role === 'assistant') return messages[i].id;
    }
    return null;
  })();

  if (isLoading && !messages.length) {
    return <div className={styles.loading}>Loading chat…</div>;
  }

  const pageTitle = chatData?.title || 'Untitled Chat';

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.titleInfo}>
          <h2 className={styles.title}>{pageTitle}</h2>
          {isCeremonyThread && (
            <span className={styles.titleSubtitle}>Your first conversation.</span>
          )}
          {projectId && <StatusBadge label={`Project: ${projectId}`} />}
        </div>
      </div>

      <div className={styles.timelineWrap}>
        <div
          className={styles.timeline}
          ref={scrollRef}
          onScroll={handleScroll}
          aria-live="polite"
          aria-label="Conversation"
        >
          <div className={styles.timelineInner}>
            {displayMessages.map((msg) => {
              const isStreamingTail =
                (status === 'streaming' ||
                  (isCeremonyThread && ceremonyStreamActive)) &&
                msg.role === 'assistant' &&
                msg.id === messages.at(-1)?.id;
              return (
                <ChatMessage
                  key={msg.id}
                  message={{
                    ...msg,
                    streaming: isStreamingTail,
                    toolParts:
                      msg.role === 'assistant'
                        ? isStreamingTail && liveToolParts.length
                          ? liveToolParts
                          : persistedToolParts(msg.id)
                        : undefined,
                    variant:
                      msg.role === 'assistant' && !isCeremonyThread
                        ? variantInfo(
                            chatData?.messages?.find((x) => (x.id ?? '') === msg.id)
                              ?.metadata as Record<string, unknown> | undefined,
                          )
                        : undefined,
                  }}
                  isNaviActive={isWorking}
                  feedback={msg.role === 'assistant' ? feedbackFor(msg.id) : undefined}
                  onFeedback={
                    msg.role === 'assistant'
                      ? (rating) => handleFeedback(msg.id, rating)
                      : undefined
                  }
                  onEditResend={
                    msg.role === 'user'
                      ? (content) => handleEditResend(msg.id, content)
                      : undefined
                  }
                  onRegenerate={
                    msg.role === 'assistant' && msg.id === lastAssistantId && !isWorking
                      ? handleRegenerate
                      : undefined
                  }
                  onContinue={
                    msg.role === 'assistant' && msg.id === lastAssistantId && !isWorking
                      ? handleContinue
                      : undefined
                  }
                  onSelectVariant={
                    msg.role === 'assistant'
                      ? (index) => handleSelectVariant(msg.id, index)
                      : undefined
                  }
                  onCeremonySelect={
                    isCeremonyThread && msg.role === 'assistant'
                      ? handleCeremonySelect
                      : undefined
                  }
                />
              );
            })}

            {showTyping && <TypingIndicator isNaviActive={isWorking} />}

            {displayMessages.length === 0 && (
              <WelcomeScreen
                disabled={status === 'submitted' || status === 'streaming'}
                onSelectPrompt={(text) => {
                  setSendError(null);
                  void sendMessage({ text });
                }}
              />
            )}
          </div>
        </div>
        {showScrollFab && (
          <button
            className={styles.scrollFab}
            onClick={scrollToBottom}
            type="button"
            aria-label="Scroll to bottom"
          >
            <ArrowDown size={18} />
          </button>
        )}
      </div>

      <div className={styles.composerArea}>
        {(sendError ?? streamError) && (
          <div className={styles.errorBanner} role="alert">
            <AlertCircle size={15} />
            <span>{sendError ?? streamError?.message}</span>
          </div>
        )}
        <ChatInput
          value={inputText}
          onChange={setInputText}
          onSend={handleSend}
          status={status as 'ready' | 'submitted' | 'streaming'}
          onStop={stop}
        />
        <div className={styles.composerHint}>
          Enter to send · Shift+Enter for a new line
          {status === 'streaming' && ' · Click Send to stop'}
        </div>
      </div>
    </div>
  );
}

// ─── Draft chat composer (unchanged core logic) ────────────────────────────────

type DraftDisplayMessage = ChatMessageType & {
  id: string;
  role: 'user' | 'assistant' | 'system' | 'tool';
  content: string;
  createdAt?: string;
  pending?: boolean;
  failed?: boolean;
  streaming?: boolean;
  toolParts?: ToolPart[];
};

const TERMINAL_EVENT_TYPES = new Set([
  'assistant.message.completed',
  'run.completed',
  'run.failed',
  'run.cancelled',
]);

function DraftChatComposer({
  freshKey,
  composeText,
  projectId,
  navigate,
}: {
  freshKey: string | null;
  composeText: string;
  projectId?: string;
  navigate: (path: string, replace?: boolean) => void;
}) {
  const { setLocalWorking } = useLocalWorking();
  const [provisionalChatId, setProvisionalChatId] = useState<string | null>(
    null,
  );
  const [awaitingNaviResponse, setAwaitingNaviResponse] = useState(false);
  const streamChatId = provisionalChatId;
  const { data: chatData } = useChatThread(streamChatId, {
    refetchInterval: awaitingNaviResponse ? 3_000 : 15_000,
  });
  const draftLive = useLiveEvents({
    chatId: streamChatId ?? undefined,
    enabled: Boolean(streamChatId),
  });
  const queryClient = useQueryClient();
  const [input, setInput] = useState('');
  const [sending, setSending] = useState(false);
  const [sendError, setSendError] = useState<string | null>(null);
  const [pendingMessages, setPendingMessages] = useState<DraftDisplayMessage[]>(
    [],
  );
  const [assistantDraft, setAssistantDraft] =
    useState<DraftDisplayMessage | null>(null);
  // Soft "taking longer than usual" notice shown when the wait budget elapses.
  // Unlike sendError, this does NOT discard the (already-persisted) chat — the
  // live-event finalize path still completes the turn when NAVI replies.
  const [slowResponseNotice, setSlowResponseNotice] = useState<string | null>(
    null,
  );
  const scrollRef = useRef<HTMLDivElement>(null);
  const processedLiveEvents = useRef<Set<string>>(new Set());
  const provisionalChatIdRef = useRef<string | null>(null);
  const atBottomRef = useRef(true);
  const [showScrollFab, setShowScrollFab] = useState(false);

  useEffect(() => {
    provisionalChatIdRef.current = provisionalChatId;
  }, [provisionalChatId]);

  const messages = useMemo(() => {
    const serverMessages = Array.isArray(chatData?.messages)
      ? (chatData.messages
          .map(normalizeDraftMessage)
          .filter(Boolean) as DraftDisplayMessage[])
      : [];
    const serverKeys = new Set(serverMessages.map(msgFingerprint));
    const unsyncedPending = pendingMessages.filter(
      (msg) => msg.failed || !serverKeys.has(msgFingerprint(msg)),
    );
    return assistantDraft
      ? [...serverMessages, ...unsyncedPending, assistantDraft]
      : [...serverMessages, ...unsyncedPending];
  }, [assistantDraft, chatData?.messages, pendingMessages]);

  const isWorking =
    sending || awaitingNaviResponse || Boolean(assistantDraft);

  useEffect(() => {
    setLocalWorking(isWorking);
    return () => setLocalWorking(false);
  }, [isWorking, setLocalWorking]);

  const showTyping = isWorking && messages.at(-1)?.role !== 'assistant';

  const finalizeDraftConversation = useCallback(
    async (id: string) => {
      await refreshChatQueries(queryClient, id, projectId, true);
      const path = projectId
        ? `/projects/${projectId}/chats/${id}`
        : `/chats/${id}`;
      navigate(path, true);
      setAwaitingNaviResponse(false);
      setProvisionalChatId(null);
      setSlowResponseNotice(null);
    },
    [navigate, projectId, queryClient],
  );

  const rollbackProvisionalChat = useCallback(
    async (id: string, message?: string) => {
      try {
        await archiveChat(id);
      } catch (err) {
        console.error('Failed to archive provisional chat', err);
      }
      setProvisionalChatId(null);
      setAwaitingNaviResponse(false);
      setSlowResponseNotice(null);
      if (message) setSendError(message);
    },
    [],
  );

  useEffect(() => {
    const el = scrollRef.current;
    if (el && atBottomRef.current) {
      el.scrollTop = el.scrollHeight;
    }
  }, [messages.length, assistantDraft?.content]);

  const handleScroll = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    const dist = el.scrollHeight - el.scrollTop - el.clientHeight;
    atBottomRef.current = dist < 80;
    setShowScrollFab(dist > 200);
  }, []);

  const scrollToBottom = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' });
    atBottomRef.current = true;
    setShowScrollFab(false);
  }, []);

  useEffect(() => {
    processedLiveEvents.current.clear();
    setAssistantDraft(null);
    setPendingMessages([]);
    setSendError(null);
    setSlowResponseNotice(null);
  }, [freshKey]);

  useEffect(() => {
    const previousId = provisionalChatIdRef.current;
    if (previousId) {
      void archiveChat(previousId).catch((err) =>
        console.error('Failed to archive provisional chat when opening new draft', err),
      );
    }
    setProvisionalChatId(null);
    setAwaitingNaviResponse(false);
    setAssistantDraft(null);
    setPendingMessages([]);
    setSendError(null);
    setInput(composeText);
  }, [freshKey, composeText]);

  useEffect(() => {
    const liveChatId = streamChatId;
    if (!liveChatId) return;
    for (const item of draftLive.events) {
      if (processedLiveEvents.current.has(item.id)) continue;
      processedLiveEvents.current.add(item.id);
      handleLiveEvent(item, {
        chatId: liveChatId,
        projectId,
        setAssistantDraft,
        refreshChat: () =>
          refreshChatQueries(queryClient, liveChatId, projectId, false),
        onNaviResponded:
          awaitingNaviResponse && provisionalChatId === liveChatId
            ? () => void finalizeDraftConversation(liveChatId)
            : undefined,
        onRunTerminalFailure:
          awaitingNaviResponse && provisionalChatId === liveChatId
            ? () => {
                if (threadHasAssistantReply(chatData?.messages ?? [])) {
                  void finalizeDraftConversation(liveChatId);
                } else {
                  void rollbackProvisionalChat(
                    liveChatId,
                    'NAVI did not respond. Try again.',
                  );
                }
              }
            : undefined,
      });
    }
  }, [
    awaitingNaviResponse,
    chatData?.messages,
    draftLive.events,
    finalizeDraftConversation,
    projectId,
    provisionalChatId,
    queryClient,
    rollbackProvisionalChat,
    streamChatId,
  ]);

  useEffect(() => {
    if (!awaitingNaviResponse || !provisionalChatId || !chatData?.messages)
      return;
    if (threadHasAssistantReply(chatData.messages)) {
      void finalizeDraftConversation(provisionalChatId);
    }
  }, [
    awaitingNaviResponse,
    chatData?.messages,
    finalizeDraftConversation,
    provisionalChatId,
  ]);

  useEffect(() => {
    if (!awaitingNaviResponse || !provisionalChatId) return;
    // The message is already persisted server-side (the provisional chat exists
    // via createChat). When our wait budget elapses we do NOT discard it —
    // doing so is what previously lost users' messages. Instead surface a soft
    // notice and keep awaiting, so the live-event / chat-poll finalize paths
    // complete the turn whenever NAVI's terminal event arrives.
    const id = window.setTimeout(() => {
      setSlowResponseNotice(
        'NAVI is taking longer than usual to respond. Your message is saved — this will update automatically when NAVI replies.',
      );
    }, DRAFT_RESPONSE_TIMEOUT_MS);
    return () => window.clearTimeout(id);
  }, [awaitingNaviResponse, provisionalChatId]);

  const handleSend = async (messageText: string = input) => {
    const textToSend = messageText.trim();
    if (!textToSend || sending || awaitingNaviResponse) return;
    const tempId = `pending-${crypto.randomUUID()}`;
    const optimisticMessage: DraftDisplayMessage = {
      id: tempId,
      role: 'user',
      content: textToSend,
      createdAt: new Date().toISOString(),
      pending: true,
    };

    setSending(true);
    setSendError(null);
    setSlowResponseNotice(null);
    setInput('');
    setPendingMessages((prev) => [
      ...prev.filter((msg) => !msg.failed),
      optimisticMessage,
    ]);

    let activeChatId = provisionalChatId;
    let createdProvisionalId: string | null = null;

    try {
      if (!activeChatId) {
        const created = await createChat(projectId, textToSend);
        activeChatId = created.chat_id;
        if (!activeChatId) throw new Error('Chat was created without an id');
        createdProvisionalId = activeChatId;
        setProvisionalChatId(activeChatId);
      }

      await sendChatMessage(activeChatId, textToSend);

      setAwaitingNaviResponse(true);
      await refreshChatQueries(queryClient, activeChatId, projectId, false);
    } catch (e) {
      const orphanId =
        createdProvisionalId ??
        (activeChatId && !provisionalChatId ? activeChatId : null);
      if (orphanId) await rollbackProvisionalChat(orphanId);
      setSendError(errorMessage(e));
      setInput(textToSend);
      setPendingMessages((prev) =>
        prev.map((msg) =>
          msg.id === tempId ? { ...msg, pending: false, failed: true } : msg,
        ),
      );
    } finally {
      setSending(false);
    }
  };


  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.titleInfo}>
          <h2 className={styles.title}>New conversation</h2>
          {projectId && <StatusBadge label={`Project: ${projectId}`} />}
        </div>
      </div>

      <div className={styles.timelineWrap}>
        <div
          className={styles.timeline}
          ref={scrollRef}
          onScroll={handleScroll}
          aria-live="polite"
          aria-label="Conversation"
        >
          <div className={styles.timelineInner}>
            {messages.map((msg) => (
              <ChatMessage
                key={msg.id}
                message={{
                  id: msg.id,
                  role: msg.role === 'user' ? 'user' : 'assistant',
                  content: msg.content,
                  createdAt: msg.createdAt,
                  pending: msg.pending,
                  failed: msg.failed,
                  streaming: msg.streaming,
                  toolParts:
                    msg.role === 'assistant'
                      ? msg.toolParts ?? parseToolParts((msg.metadata as Record<string, unknown> | undefined)?.toolParts)
                      : undefined,
                }}
                isNaviActive={isWorking}
                onRetry={msg.failed ? () => void handleSend(msg.content) : undefined}
              />
            ))}

            {showTyping && <TypingIndicator isNaviActive={isWorking} />}

            {messages.length === 0 && (
              <WelcomeScreen
                disabled={sending || awaitingNaviResponse}
                onSelectPrompt={(text) => void handleSend(text)}
              />
            )}
          </div>
        </div>
        {showScrollFab && (
          <button
            className={styles.scrollFab}
            onClick={scrollToBottom}
            type="button"
            aria-label="Scroll to bottom"
          >
            <ArrowDown size={18} />
          </button>
        )}
      </div>

      <div className={styles.composerArea}>
        {slowResponseNotice && !sendError && (
          <div className={styles.noticeBanner} role="status">
            <Loader2 size={15} className={styles.spinner} />
            <span>{slowResponseNotice}</span>
          </div>
        )}
        {sendError && (
          <div className={styles.errorBanner} role="alert">
            <AlertCircle size={15} />
            <span>{sendError}</span>
          </div>
        )}
        <ChatInput
          value={input}
          onChange={setInput}
          onSend={handleSend}
          status={awaitingNaviResponse ? 'submitted' : sending ? 'submitted' : 'ready'}
        />
        <div className={styles.composerHint}>
          Enter to send · Shift+Enter for a new line
        </div>
      </div>
    </div>
  );
}

// ─── Shared UI sub-components ─────────────────────────────────────────────────

function WelcomeScreen({
  disabled,
  onSelectPrompt,
}: {
  disabled: boolean;
  onSelectPrompt: (text: string) => void;
}) {
  return (
    <div className={styles.welcomeContainer}>
      <div className={styles.welcomeHeader}>
        <div className={styles.welcomeLogo}>
          <Bot size={32} className={styles.botGlow} />
        </div>
        <h3 className={styles.welcomeTitle}>Welcome to NAVI Chat</h3>
        <p className={styles.welcomeSubtitle}>
          NAVI is ready to orchestrate, research, and execute tasks. Try one of
          the quick starters below.
        </p>
      </div>
      <div className={styles.starterGrid}>
        {STARTER_PROMPTS.map((starter, index) => (
          <button
            key={index}
            className={styles.starterCard}
            onClick={() => onSelectPrompt(starter.prompt)}
            disabled={disabled}
          >
            <div className={styles.starterCardHeader}>
              <Sparkles size={14} className={styles.starterIcon} />
              <span className={styles.starterCardTitle}>{starter.title}</span>
            </div>
            <p className={styles.starterCardDesc}>{starter.desc}</p>
          </button>
        ))}
      </div>
    </div>
  );
}

// ─── Utilities ────────────────────────────────────────────────────────────────

function getMessageText(msg: UIMessage): string {
  return msg.parts
    .filter((p): p is { type: 'text'; text: string } => p.type === 'text')
    .map((p) => p.text)
    .join('');
}

function toUIMessage(msg: ChatMessageType, index: number): UIMessage | null {
  const view = toChatMessageView(msg, index);
  if (!view) return null;
  return {
    id: view.id,
    role: view.role,
    parts: [{ type: 'text' as const, text: view.content }],
  };
}

function toChatMessageView(msg: ChatMessageType, index: number): ChatMessageView | null {
  const content = typeof msg.content === 'string' ? msg.content.trim() : '';
  if (!content) return null;
  const role = normalizeRole(msg.role);
  if (role !== 'user' && role !== 'assistant') return null;
  const meta = msg.metadata as Record<string, unknown> | undefined;
  return {
    id: msg.id ?? `server-${index}`,
    role,
    content,
    createdAt: msg.createdAt ?? msg.created_at,
    renderPayload: parseRenderPayload(meta) ?? undefined,
    ceremonyStep: meta?.ceremonyStep as string | undefined,
    ceremonyType: meta?.ceremonyType as string | undefined,
    ceremonyControls: meta?.ceremonyControls as CeremonyControlOption[] | undefined,
    ceremonyActions: meta?.ceremonyActions as CeremonyControlAction[] | undefined,
    ceremonyDefaults: meta?.ceremonyDefaults as Record<string, boolean> | undefined,
  };
}

function uiMessageToChatView(msg: UIMessage): ChatMessageView | null {
  const content = getMessageText(msg).trim();
  if (!content) return null;
  const role = msg.role === 'user' ? 'user' : 'assistant';
  return { id: msg.id, role, content };
}

function normalizeDraftMessage(
  msg: ChatMessageType,
  index: number,
): DraftDisplayMessage | null {
  const content = typeof msg.content === 'string' ? msg.content : '';
  if (!content) return null;
  const createdAt = msg.createdAt ?? msg.created_at;
  return {
    ...msg,
    id: msg.id ?? `${createdAt ?? 'message'}-${index}`,
    role: normalizeRole(msg.role) as DraftDisplayMessage['role'],
    content,
    createdAt,
  };
}

function normalizeRole(role?: string): string {
  switch ((role ?? '').toLowerCase()) {
    case 'owner':
    case 'user':
      return 'user';
    case 'assistant':
    case 'navi':
      return 'assistant';
    case 'system':
      return 'system';
    case 'tool':
      return 'tool';
    default:
      return 'assistant';
  }
}

function msgFingerprint(msg: { role: string; content: string }): string {
  return `${msg.role}:${msg.content}`;
}

function refreshChatQueries(
  queryClient: ReturnType<typeof useQueryClient>,
  chatId: string,
  projectId?: string,
  includeLists = true,
) {
  return Promise.all([
    queryClient.invalidateQueries({ queryKey: ['chat', chatId] }),
    queryClient.invalidateQueries({
      queryKey: ['chat-runtime-summary', chatId],
    }),
    includeLists
      ? queryClient.invalidateQueries({ queryKey: ['chats'] })
      : Promise.resolve(),
    includeLists && projectId
      ? queryClient.invalidateQueries({
          queryKey: ['project-chats', projectId],
        })
      : Promise.resolve(),
  ]);
}

function threadHasAssistantReply(messages: ChatMessageType[]): boolean {
  return messages.some((msg) => {
    const role = (msg.role ?? '').toLowerCase();
    const content =
      typeof msg.content === 'string' ? msg.content.trim() : '';
    return (role === 'assistant' || role === 'navi') && content.length > 0;
  });
}

const RUN_FAILURE_EVENT_TYPES = new Set(['run.failed', 'run.cancelled']);

function handleLiveEvent(
  item: LiveEvent,
  options: {
    chatId: string;
    projectId?: string;
    setAssistantDraft: React.Dispatch<
      React.SetStateAction<DraftDisplayMessage | null>
    >;
    refreshChat: () => Promise<unknown>;
    onNaviResponded?: () => void;
    onRunTerminalFailure?: () => void;
  },
) {
  const event = liveEventRecord(item.frame);
  if (!event) return;
  const type = stringValue(event.type);
  const payload = recordValue(event.payload);
  if (!type || !eventBelongsToChat(event, payload, options.chatId)) return;

  if (type === 'assistant.message.partial') {
    const content = stringValue(payload?.content);
    if (!content) return;
    const runId =
      stringValue(payload?.run_id) ??
      stringValue(event.run_id) ??
      'live';
    options.setAssistantDraft((prev) => ({
      id: `draft-${runId}`,
      role: 'assistant',
      content,
      createdAt: stringValue(event.timestamp),
      streaming: true,
      toolParts: prev?.toolParts,
    }));
    return;
  }

  if (type === 'tool.call.started' || type === 'tool.call.completed' || type === 'tool.call.failed') {
    const callId = stringValue(payload?.call_id);
    if (!callId) return;
    const runId =
      stringValue(payload?.run_id) ?? stringValue(event.run_id) ?? 'live';
    const toolName = stringValue(payload?.tool_name) ?? 'tool';
    const resolved = type !== 'tool.call.started';
    const isError = type === 'tool.call.failed';
    const result = isError ? stringValue(payload?.error) : toolResultValue(payload?.result);
    options.setAssistantDraft((prev) => {
      const base: DraftDisplayMessage = prev ?? {
        id: `draft-${runId}`,
        role: 'assistant',
        content: '',
        createdAt: stringValue(event.timestamp),
        streaming: true,
      };
      const parts = base.toolParts ? base.toolParts.slice() : [];
      const idx = parts.findIndex((p) => p.toolInvocationId === callId);
      const next: ToolPart = {
        toolInvocationId: callId,
        toolName,
        state: resolved ? 'result' : 'call',
        result: result ?? (idx >= 0 ? parts[idx].result : undefined),
        isError,
      };
      if (idx >= 0) parts[idx] = next;
      else parts.push(next);
      return { ...base, toolParts: parts };
    });
    return;
  }

  if (type === 'assistant.message.completed') {
    options.setAssistantDraft(null);
    void options.refreshChat();
    options.onNaviResponded?.();
    return;
  }

  if (RUN_FAILURE_EVENT_TYPES.has(type)) {
    options.onRunTerminalFailure?.();
    return;
  }

  if (TERMINAL_EVENT_TYPES.has(type) || type === 'message.received') {
    void options.refreshChat();
  }
}

function eventBelongsToChat(
  event: Record<string, unknown>,
  payload: Record<string, unknown> | null,
  chatId: string,
): boolean {
  const eventChatId =
    stringValue(event.chat_id) ??
    stringValue(event.correlation_id) ??
    stringValue(payload?.chat_id) ??
    stringValue(payload?.runtime_session_id);
  return eventChatId === chatId;
}

function liveEventRecord(
  frame: LiveFrame,
): Record<string, unknown> | null {
  if (!frame || typeof frame !== 'object') return null;
  if ('event' in frame) {
    return recordValue((frame as { event?: unknown }).event);
  }
  return null;
}

function recordValue(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== 'object') return null;
  return value as Record<string, unknown>;
}

function stringValue(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value : undefined;
}

function toolResultValue(value: unknown): unknown {
  if (typeof value === 'string') return value.trim() ? value : undefined;
  if (value && typeof value === 'object' && !Array.isArray(value)) return value;
  return undefined;
}

// parseToolParts coerces persisted/raw metadata into a clean ToolPart[] (or
// undefined when there are none), dropping malformed entries.
function parseToolParts(raw: unknown): ToolPart[] | undefined {
  if (!Array.isArray(raw)) return undefined;
  const parts: ToolPart[] = [];
  for (const entry of raw) {
    if (!entry || typeof entry !== 'object') continue;
    const rec = entry as Record<string, unknown>;
    const toolInvocationId =
      stringValue(rec.toolInvocationId) ?? stringValue(rec.call_id) ?? '';
    const toolName = stringValue(rec.toolName) ?? stringValue(rec.tool_name) ?? 'tool';
    if (!toolInvocationId) continue;
    parts.push({
      toolInvocationId,
      toolName,
      state: stringValue(rec.state) ?? 'result',
      result: toolResultValue(rec.result),
      isError: rec.isError === true,
    });
  }
  return parts.length ? parts : undefined;
}

function variantInfo(raw: unknown): { selectedIndex: number; total: number } | undefined {
  if (!raw || typeof raw !== 'object') return undefined;
  const rec = raw as Record<string, unknown>;
  const total = numberValue(rec.variantCount);
  if (!total || total < 2) return undefined;
  return {
    selectedIndex: numberValue(rec.selectedVariantIndex) ?? 0,
    total,
  };
}

function numberValue(value: unknown): number | undefined {
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string' && value.trim()) {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return undefined;
}

function errorMessage(err: unknown): string {
  if (err instanceof Error && err.message) return err.message;
  return 'Message failed to send. Your draft is still here.';
}


