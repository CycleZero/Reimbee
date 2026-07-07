import { create } from 'zustand';
import { generateUUIDv7 } from '@/utils/uuid';

// ============================================
// 对话消息 + SSE 连接状态管理
// ============================================

type MessageRole = 'user' | 'assistant';

export interface ToolCallRecord {
  id: string;
  toolName: string;
  status: 'running' | 'success' | 'error';
  input?: unknown;
  output?: unknown;
  errorMessage?: string;
}

export interface ChatMessage {
  id: string;
  role: MessageRole;
  content: string;
  timestamp: number;
  isStreaming?: boolean;
  toolCalls?: ToolCallRecord[];
}

type ReimbPhase = 'idle' | 'phase1_collect' | 'phase2_validate' | 'phase3_execute';

export interface ConfirmPrompt {
  action: string;
  prompt: string;
  context?: unknown;
}

export interface SessionItem {
  id: string;
  title: string;
  updatedAt: number;
}

interface ChatState {
  sessionId: string | null;
  messages: ChatMessage[];
  connectionStatus: 'disconnected' | 'connecting' | 'connected' | 'error';
  isThinking: boolean;
  thinkingMessage: string;
  currentStreamingMessageId: string | null;
  currentPhase: ReimbPhase;
  confirmPrompt: ConfirmPrompt | null;
  sessions: SessionItem[];

  initSession: (sessionId?: string) => string;
  addUserMessage: (content: string) => void;
  startStreamingMessage: () => string;
  appendStreamContent: (messageId: string, chunk: string) => void;
  finishStreamingMessage: (messageId: string) => void;
  setThinking: (message: string) => void;
  clearThinking: () => void;
  addToolCall: (messageId: string, call: ToolCallRecord) => void;
  updateToolCall: (
    messageId: string,
    callId: string,
    update: Partial<ToolCallRecord>,
  ) => void;
  setPhase: (phase: ReimbPhase) => void;
  setConfirmPrompt: (prompt: ConfirmPrompt | null) => void;
  setConnectionStatus: (status: ChatState['connectionStatus']) => void;
  clearMessages: () => void;
  updateSessionTitle: (sessionId: string, title: string) => void;
  removeSession: (sessionId: string) => void;
}

export const useChatStore = create<ChatState>()((set, get) => ({
  sessionId: null,
  messages: [],
  connectionStatus: 'disconnected',
  isThinking: false,
  thinkingMessage: '',
  currentStreamingMessageId: null,
  currentPhase: 'idle',
  confirmPrompt: null,
  sessions: [],

  initSession: (sessionId) => {
    const id = sessionId ?? generateUUIDv7();
    set((s) => ({
      sessionId: id,
      messages: [],
      currentPhase: 'idle',
      sessions: s.sessions.some((x) => x.id === id)
        ? s.sessions
        : [...s.sessions, { id, title: '新对话', updatedAt: Date.now() }],
    }));
    return id;
  },

  addUserMessage: (content) =>
    set((s) => ({
      messages: [
        ...s.messages,
        { id: generateUUIDv7(), role: 'user', content, timestamp: Date.now() },
      ],
    })),

  startStreamingMessage: () => {
    const id = generateUUIDv7();
    set((s) => ({
      messages: [
        ...s.messages,
        { id, role: 'assistant', content: '', timestamp: Date.now(), isStreaming: true, toolCalls: [] },
      ],
      currentStreamingMessageId: id,
    }));
    return id;
  },

  appendStreamContent: (messageId, chunk) =>
    set((s) => ({
      messages: s.messages.map((m) =>
        m.id === messageId ? { ...m, content: m.content + chunk } : m,
      ),
    })),

  finishStreamingMessage: (messageId) =>
    set((s) => ({
      messages: s.messages.map((m) =>
        m.id === messageId ? { ...m, isStreaming: false } : m,
      ),
      currentStreamingMessageId: null,
    })),

  setThinking: (message) => set({ isThinking: true, thinkingMessage: message }),
  clearThinking: () => set({ isThinking: false, thinkingMessage: '' }),

  addToolCall: (messageId, call) =>
    set((s) => ({
      messages: s.messages.map((m) =>
        m.id === messageId
          ? { ...m, toolCalls: [...(m.toolCalls ?? []), call] }
          : m,
      ),
    })),

  updateToolCall: (messageId, callId, update) =>
    set((s) => ({
      messages: s.messages.map((m) =>
        m.id === messageId
          ? {
              ...m,
              toolCalls: m.toolCalls?.map((tc) =>
                tc.id === callId ? { ...tc, ...update } : tc,
              ),
            }
          : m,
      ),
    })),

  setPhase: (phase) => set({ currentPhase: phase }),
  setConfirmPrompt: (prompt) => set({ confirmPrompt: prompt }),
  setConnectionStatus: (status) => set({ connectionStatus: status }),

  clearMessages: () =>
    set((s) => ({
      messages: [],
      currentPhase: 'idle',
      sessions: s.sessionId
        ? s.sessions.map((x) =>
            x.id === s.sessionId ? { ...x, updatedAt: Date.now() } : x,
          )
        : s.sessions,
    })),

  updateSessionTitle: (sessionId, title) =>
    set((s) => ({
      sessions: s.sessions.map((x) =>
        x.id === sessionId ? { ...x, title } : x,
      ),
    })),

  removeSession: (sessionId) =>
    set((s) => ({
      sessions: s.sessions.filter((x) => x.id !== sessionId),
    })),
}));
