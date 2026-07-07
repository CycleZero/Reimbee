// ============================================
// Chat Store — Zustand 单一状态树
// 组织为逻辑分组（消息 / 流式 / 会话 / UI 状态）
// 渲染器注册表不走 Store（见 registry.ts）
// ============================================

import { create } from 'zustand';
import { generateUUIDv7 } from '@/utils/uuid';
import type {
  ChatMessage,
  ToolCallRecord,
  ReimbPhase,
  ConfirmPrompt,
  SessionItem,
  ConnectionStatus,
} from '../types';

// ============================================
// State
// ============================================

interface ChatState {
  // ---- 消息 ----
  messages: ChatMessage[];
  currentStreamingMessageId: string | null;

  // ---- 流式连接 ----
  connectionStatus: ConnectionStatus;

  // ---- 思考状态 ----
  isThinking: boolean;
  thinkingMessage: string;

  // ---- 阶段 ----
  currentPhase: ReimbPhase;

  // ---- 确认 ----
  confirmPrompt: ConfirmPrompt | null;

  // ---- 会话 ----
  sessions: SessionItem[];
  currentSessionId: string | null;

  // ---- Actions ----
  // 消息
  addUserMessage: (content: string) => void;
  startStreamingMessage: () => string;
  appendStreamContent: (messageId: string, chunk: string) => void;
  finishStreamingMessage: (messageId: string) => void;

  // 工具调用
  addToolCall: (messageId: string, call: ToolCallRecord) => void;
  updateToolCall: (messageId: string, callId: string, update: Partial<ToolCallRecord>) => void;

  // 连接
  setConnectionStatus: (status: ConnectionStatus) => void;

  // 思考
  setThinking: (message: string) => void;
  clearThinking: () => void;

  // 阶段
  setPhase: (phase: ReimbPhase) => void;

  // 确认
  setConfirmPrompt: (prompt: ConfirmPrompt | null) => void;

  // 会话
  initSession: (sessionId?: string) => string;
  clearMessages: () => void;
  updateSessionTitle: (sessionId: string, title: string) => void;
  removeSession: (sessionId: string) => void;
}

// ============================================
// Store
// ============================================

export const useChatStore = create<ChatState>()((set, get) => ({
  // ---- 消息 ----
  messages: [],
  currentStreamingMessageId: null,

  // ---- 流式连接 ----
  connectionStatus: 'disconnected',

  // ---- 思考状态 ----
  isThinking: false,
  thinkingMessage: '',

  // ---- 阶段 ----
  currentPhase: 'idle',

  // ---- 确认 ----
  confirmPrompt: null,

  // ---- 会话 ----
  sessions: [],
  currentSessionId: null,

  // ============================================
  // 消息 Actions
  // ============================================

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
        {
          id,
          role: 'assistant',
          content: '',
          timestamp: Date.now(),
          isStreaming: true,
          toolCalls: [],
        },
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

  // ============================================
  // 工具调用 Actions
  // ============================================

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

  // ============================================
  // 连接 Actions
  // ============================================

  setConnectionStatus: (status) => set({ connectionStatus: status }),

  // ============================================
  // 思考 Actions
  // ============================================

  setThinking: (message) => set({ isThinking: true, thinkingMessage: message }),
  clearThinking: () => set({ isThinking: false, thinkingMessage: '' }),

  // ============================================
  // 阶段 Actions
  // ============================================

  setPhase: (phase) => set({ currentPhase: phase }),

  // ============================================
  // 确认 Actions
  // ============================================

  setConfirmPrompt: (prompt) => set({ confirmPrompt: prompt }),

  // ============================================
  // 会话 Actions
  // ============================================

  initSession: (sessionId) => {
    const id = sessionId ?? generateUUIDv7();
    set((s) => ({
      currentSessionId: id,
      messages: [],
      currentPhase: 'idle',
      sessions: s.sessions.some((x) => x.id === id)
        ? s.sessions
        : [...s.sessions, { id, title: '新对话', updatedAt: Date.now() }],
    }));
    return id;
  },

  clearMessages: () =>
    set((s) => ({
      messages: [],
      currentPhase: 'idle',
      sessions: s.currentSessionId
        ? s.sessions.map((x) =>
            x.id === s.currentSessionId ? { ...x, updatedAt: Date.now() } : x,
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
