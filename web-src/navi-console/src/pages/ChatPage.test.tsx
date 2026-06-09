import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ChatPage } from './ChatPage';

// --- React Hook Mocking for ES Modules ---
let nullCount = 0;
(globalThis as any).__mockProvisionalChatId = null;
(globalThis as any).__resetNullCount = () => {
  nullCount = 0;
};

vi.mock('react', async (importOriginal) => {
  const original = await importOriginal<any>();
  return {
    ...original,
    useState: (init: any) => {
      if (init === null && (globalThis as any).__mockProvisionalChatId) {
        nullCount++;
        const current = nullCount;
        if (nullCount >= 4) {
          nullCount = 0;
        }
        if (current === 1) {
          const [, setter] = original.useState((globalThis as any).__mockProvisionalChatId);
          return [(globalThis as any).__mockProvisionalChatId, setter];
        }
      }
      return original.useState(init);
    },
  };
});

// --- Mocks ---
const mockNavigate = vi.fn();
const mockQuery = new URLSearchParams();
vi.mock('@/app/router', () => ({
  useNavigate: () => mockNavigate,
  useRoute: () => ({
    params: { chatId: 'new' },
    query: mockQuery,
    route: 'chats',
  }),
}));

const mockCreateChat = vi.fn();
const mockSendChatMessage = vi.fn();
const mockArchiveChat = vi.fn();
const mockUseChatThread = vi.fn();
const mockUseChatRuntimeSummary = vi.fn();

vi.mock('@/api/chats', () => ({
  createChat: (...args: any[]) => mockCreateChat(...args),
  sendChatMessage: (...args: any[]) => mockSendChatMessage(...args),
  archiveChat: (...args: any[]) => mockArchiveChat(...args),
  useChatThread: (...args: any[]) => mockUseChatThread(...args),
  useChatRuntimeSummary: (...args: any[]) => mockUseChatRuntimeSummary(...args),
  sendMessageFeedback: vi.fn(),
  editAndResendMessage: vi.fn(),
  regenerateLastReply: vi.fn(),
  continueLastReply: vi.fn(),
  selectMessageVariant: vi.fn(),
}));

vi.mock('@/hooks/LiveEventsContext', () => ({
  useLiveEventsContext: () => ({
    connectionState: 'connected',
    events: [],
  }),
}));

vi.mock('@/hooks/useLiveEvents', () => ({
  useLiveEvents: () => ({
    connectionState: 'connected',
    events: [],
  }),
}));

vi.mock('@/api/ceremony', () => ({
  useCeremony: () => ({ data: null, isLoading: false }),
  useCeremonyStep: () => ({ mutate: vi.fn(), data: null, isLoading: false }),
}));

vi.mock('@/components/shell/AppShell', () => ({
  useRail: () => ({
    collapsed: false,
    setCollapsed: vi.fn(),
  }),
  useLocalWorking: () => ({
    localWorking: false,
    setLocalWorking: vi.fn(),
  }),
}));

vi.mock('@/components/chat/ChatInput', () => ({
  ChatInput: ({ value, onChange, onSend }: any) => (
    <div>
      <input
        data-testid="chat-input"
        value={value || ''}
        onChange={(e) => onChange(e.target.value)}
      />
      <button data-testid="chat-send" onClick={() => onSend(value)}>
        Send
      </button>
    </div>
  ),
}));

vi.mock('@/components/chat/ChatMessage', () => ({
  ChatMessage: ({ message, onRetry }: any) => (
    <div data-testid={`msg-${message.id}`}>
      <span data-testid="msg-role">{message.role}</span>
      <span data-testid="msg-content">{message.content}</span>
      <span data-testid="msg-status">{message.failed ? 'failed' : message.pending ? 'pending' : 'sent'}</span>
      {message.failed && <button data-testid="msg-retry" onClick={onRetry}>Retry</button>}
    </div>
  ),
}));

vi.mock('@/components/chat/TypingIndicator', () => ({
  TypingIndicator: () => <div data-testid="typing-indicator">Typing...</div>,
}));

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <ChatPage chatId="new" />
    </QueryClientProvider>,
  );
}

describe('ChatPage — DraftChatComposer Flow', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockQuery.delete('compose');
    mockUseChatThread.mockReturnValue({ data: null });
    mockUseChatRuntimeSummary.mockReturnValue({ data: null });
    mockArchiveChat.mockResolvedValue({});
    (globalThis as any).__mockProvisionalChatId = null;
    (globalThis as any).__resetNullCount();
  });

  it('renders starter prompt cards when the thread has no messages', () => {
    renderPage();
    expect(screen.getByText('Welcome to NAVI Chat')).toBeInTheDocument();
    expect(screen.getByText('Analyze Runs')).toBeInTheDocument();
  });

  it('New Chat: calls createChat and then sendChatMessage with the created ID', async () => {
    mockCreateChat.mockResolvedValue({ chat_id: 'chat-123', title: 'Test Title' });
    mockSendChatMessage.mockResolvedValue({ status: 'queued', inbox_item_id: 'inbox-456' });

    renderPage();

    const input = screen.getByTestId('chat-input');
    const sendButton = screen.getByTestId('chat-send');

    fireEvent.change(input, { target: { value: 'initial prompt' } });
    fireEvent.click(sendButton);

    await waitFor(() => {
      expect(mockCreateChat).toHaveBeenCalledWith(undefined, 'initial prompt');
    });

    await waitFor(() => {
      expect(mockSendChatMessage).toHaveBeenCalledWith('chat-123', 'initial prompt');
    });

    expect(screen.getByTestId('typing-indicator')).toBeInTheDocument();
  });

  it('Existing Chat: sends subsequent messages without calling createChat again', async () => {
    mockSendChatMessage.mockResolvedValue({ status: 'queued', inbox_item_id: 'inbox-456' });

    (globalThis as any).__mockProvisionalChatId = 'chat-123';
    (globalThis as any).__resetNullCount();

    renderPage();

    const input = screen.getByTestId('chat-input');
    const sendButton = screen.getByTestId('chat-send');

    fireEvent.change(input, { target: { value: 'second prompt' } });
    fireEvent.click(sendButton);

    await waitFor(() => {
      expect(mockSendChatMessage).toHaveBeenCalledWith('chat-123', 'second prompt');
    });

    expect(mockCreateChat).not.toHaveBeenCalled();
  });

  it('Failure & Rollback: archives the provisional chat and marks the message failed when sendChatMessage fails', async () => {
    mockCreateChat.mockResolvedValue({ chat_id: 'chat-123', title: 'Test Title' });
    mockSendChatMessage.mockRejectedValue(new Error('Send failed'));

    renderPage();

    const input = screen.getByTestId('chat-input') as HTMLInputElement;
    const sendButton = screen.getByTestId('chat-send');

    fireEvent.change(input, { target: { value: 'failing prompt' } });
    fireEvent.click(sendButton);

    await waitFor(() => {
      expect(mockCreateChat).toHaveBeenCalledWith(undefined, 'failing prompt');
    });

    await waitFor(() => {
      expect(mockSendChatMessage).toHaveBeenCalledWith('chat-123', 'failing prompt');
    });

    // Verify rollback called archiveChat
    await waitFor(() => {
      expect(mockArchiveChat).toHaveBeenCalledWith('chat-123');
    });

    // Verify input text is restored
    await waitFor(() => {
      expect(input.value).toBe('failing prompt');
    });

    // Verify optimistic message is marked failed
    const failedMsg = await screen.findByText('failed');
    expect(failedMsg).toBeInTheDocument();
  });
});
