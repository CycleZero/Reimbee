// ============================================
// Chat Store — Zustand 单一状态树
// 组织为逻辑分组（消息 / 流式 / 会话 / UI 状态）
// 渲染器注册表不走 Store（见 registry.ts）
// ============================================

import { create } from 'zustand';
import { generateUUIDv7 } from '@/utils/uuid';
import { listSessions as apiListSessions, deleteSession as apiDeleteSession, getSessionMessages } from '@/api/index';
import type {
  ChatMessage,
  ToolCallRecord,
  ReimbPhase,
  ConfirmPrompt,
  InterruptPrompt,
  SessionItem,
  ConnectionStatus,
  ApprovePayload,
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

  // ---- 推理状态（DeepSeek-R1 等模型）----
  isReasoning: boolean;
  reasoningContent: string;

  // ---- 阶段 ----
  currentPhase: ReimbPhase;

  // ---- 确认 ----
  confirmPrompt: ConfirmPrompt | null;

  // ---- 中断（v4.1）----
  interruptPrompt: InterruptPrompt | null;

  // ---- approve 信号（v4.2）----
  approveSignal: { version: number; payload: ApprovePayload } | null;

  // ---- 会话 ----
  sessions: SessionItem[];
  currentSessionId: string | null;

  // ---- 本地缓存（最多 5 个会话）----
  sessionCache: Record<string, { messages: ChatMessage[] }>;
  cacheOrder: string[];

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

  // 推理
  setReasoning: (text: string) => void;
  appendReasoning: (chunk: string) => void;
  clearReasoning: () => void;

  // 阶段
  setPhase: (phase: ReimbPhase) => void;

  // 确认
  setConfirmPrompt: (prompt: ConfirmPrompt | null) => void;

  // 中断
  setInterruptPrompt: (prompt: InterruptPrompt | null) => void;

  // approve 信号
  triggerApprove: (payload: ApprovePayload) => void;

  // 会话
  initSession: (sessionId?: string) => string;
  clearMessages: () => void;
  updateSessionTitle: (sessionId: string, title: string) => void;
  removeSession: (sessionId: string) => void;
  loadSessions: () => Promise<void>;
  switchSession: (sessionId: string) => Promise<void>;
  deleteSession: (sessionId: string) => Promise<void>;
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

  // ---- 推理状态 ----
  isReasoning: false,
  reasoningContent: '',

  // ---- 阶段 ----
  currentPhase: 'idle',

  // ---- 确认 ----
  confirmPrompt: null,

  // ---- 中断 ----
  interruptPrompt: null,

  // ---- approve 信号 ----
  approveSignal: null,

  // ---- 会话 ----
  sessions: [],
  currentSessionId: null,

  // ---- 本地缓存 ----
  sessionCache: {},
  cacheOrder: [],

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
          reasoning: s.reasoningContent || undefined,
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

  setReasoning: (text) => set({ isReasoning: true, reasoningContent: text }),
  appendReasoning: (chunk) =>
    set((s) => ({
      isReasoning: true,
      reasoningContent: s.reasoningContent + chunk,
    })),
  clearReasoning: () => set({ isReasoning: false, reasoningContent: '' }),

  // ============================================
  // 阶段 Actions
  // ============================================

  setPhase: (phase) => set({ currentPhase: phase }),

  // ============================================
  // 确认 Actions
  // ============================================

  setConfirmPrompt: (prompt) => set({ confirmPrompt: prompt }),

  setInterruptPrompt: (prompt) => set({ interruptPrompt: prompt }),

  triggerApprove: (payload) =>
    set((s) => ({
      approveSignal: { version: (s.approveSignal?.version ?? 0) + 1, payload },
    })),

  // ============================================
  // 会话 Actions
  // ============================================

  initSession: (sessionId) => {
    const id = sessionId ?? generateUUIDv7();
    set((s) => ({
      currentSessionId: id,
      messages: [],
      currentPhase: 'idle',
      confirmPrompt: null,
      sessions: s.sessions.some((x) => x.id === id)
        ? s.sessions
        : [...s.sessions, {
            id,
            title: '新对话',
            messageCount: 0,
            status: 'active',
            createdAt: new Date().toISOString(),
            updatedAt: new Date().toISOString(),
          }],
    }));
    return id;
  },

  clearMessages: () =>
    set((s) => ({
      messages: [],
      currentPhase: 'idle',
      sessions: s.currentSessionId
        ? s.sessions.map((x) =>
            x.id === s.currentSessionId
              ? { ...x, updatedAt: new Date().toISOString() }
              : x,
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

  loadSessions: async () => {
    try {
      const res = await apiListSessions();
      const remote: SessionItem[] = (res?.sessions ?? []).map((item) => ({
        id: item.session_id,
        title: item.summary || '新对话',
        messageCount: item.message_count,
        status: item.status,
        createdAt: item.created_at,
        updatedAt: item.updated_at,
      }));
      // 合并远程和本地会话：保留本地新建的，去重，按更新时间倒序
      set((s) => {
        const localIds = new Set(s.sessions.map((x) => x.id));
        const merged = s.sessions.filter((x) => {
          // 本地会话若远端也有，以远端为准（数据更全）
          const remoteMatch = remote.find((r) => r.id === x.id);
          return !remoteMatch;
        });
        for (const r of remote) {
          merged.unshift(r);
        }
        merged.sort(
          (a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime(),
        );
        return { sessions: merged };
      });
    } catch {
      // 后端接口未就绪时保留本地会话
    }
  },

  switchSession: async (sessionId) => {
    const { sessionCache, cacheOrder } = get();
    let cachedMessages: ChatMessage[] = [];

    if (sessionCache[sessionId]) {
      cachedMessages = sessionCache[sessionId].messages;
    }

    set((s) => {
      const exists = s.sessions.some((x) => x.id === sessionId);
      const touched = [sessionId, ...s.cacheOrder.filter((id) => id !== sessionId)].slice(0, 5);
      return {
        currentSessionId: sessionId,
        messages: cachedMessages,
        currentPhase: 'idle',
        confirmPrompt: null,
        interruptPrompt: null,
        isThinking: false,
        thinkingMessage: '',
        isReasoning: false,
        reasoningContent: '',
        cacheOrder: touched,
        sessions: exists
          ? s.sessions
          : [
              {
                id: sessionId,
                title: '新对话',
                messageCount: 0,
                status: 'active',
                createdAt: new Date().toISOString(),
                updatedAt: new Date().toISOString(),
              },
              ...s.sessions,
            ],
      };
    });

    try {
      const res = await getSessionMessages(sessionId);
      const remoteMsgs: ChatMessage[] = (res?.messages ?? []).map((m) => ({
        id: generateUUIDv7(),
        role: m.role as 'user' | 'assistant' | 'system',
        content: m.content,
        timestamp: new Date(m.created_at).getTime(),
        toolCalls: [],
        reasoning: m.reasoning || undefined,
      }));
      const currentState = get();
      const cached = currentState.sessionCache[sessionId];
      const finalMsgs =
        cached && cached.messages.length > 0 ? cached.messages : remoteMsgs;
      set({
        messages: finalMsgs,
        sessionCache: {
          ...currentState.sessionCache,
          [sessionId]: { messages: finalMsgs },
        },
      });
    } catch {
      // 远程失败，保持缓存数据
    }
  },

  deleteSession: async (sessionId) => {
    try {
      await apiDeleteSession(sessionId);
    } catch {
      // 后端接口未就绪时仍然从本地移除
    }
    set((s) => {
      const sessions = s.sessions.filter((x) => x.id !== sessionId);
      const currentSessionId =
        s.currentSessionId === sessionId ? null : s.currentSessionId;
      const { [sessionId]: _, ...restCache } = s.sessionCache;
      return {
        sessions,
        currentSessionId,
        sessionCache: restCache,
        cacheOrder: s.cacheOrder.filter((id) => id !== sessionId),
      };
    });
  },
}));
