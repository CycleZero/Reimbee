// ============================================
// Chat Store — Zustand 单一状态树 (v5 纯卡片模式)
// 删除旧状态: thinking/reasoning/phase/confirm
// 新增卡片原子操作: appendCard / updateLastCard
// ============================================

import { create } from 'zustand';
import { generateUUIDv7 } from '@/utils/uuid';
import { listSessions as apiListSessions, deleteSession as apiDeleteSession, getSessionMessages } from '@/api/index';
import type { SessionMessageItem } from '@/api/index';
import type {
  ChatMessage,
  MessageCard,
  SessionItem,
  ConnectionStatus,
  ApprovePayload,
} from '../types';

/** 安全解析 JSON 字符串，失败返回原始字符串 */
function safeJsonParse(s: string): unknown {
  try {
    return JSON.parse(s);
  } catch {
    return s;
  }
}

/** 将后端历史消息转换为前端 ChatMessage（含卡片拆解） */
function convertHistoryMessage(m: SessionMessageItem): ChatMessage {
  const cards: MessageCard[] = [];

  // reasoning → thinking 卡片
  if (m.reasoning) {
    cards.push({
      type: 'thinking',
      content: m.reasoning,
      thinkingText: '思考中...',
    });
  }

  // content → message 卡片
  if (m.content) {
    cards.push({
      type: 'message',
      content: m.content,
    });
  }

  // role=tool 时：tool_calls[] → 各一张 tool 卡片
  const toolCalls = m.tool_calls;
  if (m.role === 'tool' && toolCalls && toolCalls.length > 0) {
    for (const tc of toolCalls) {
      cards.push({
        type: 'tool',
        toolName: tc.name,
        status: 'success',
        input: safeJsonParse(tc.arguments),
        output: tc.result ? safeJsonParse(tc.result) : undefined,
      });
    }
  }

  return {
    id: generateUUIDv7(),
    role: m.role === 'tool' ? 'assistant' : (m.role as 'user' | 'assistant' | 'system'),
    content: m.content,
    timestamp: new Date(m.created_at).getTime(),
    cards,
    reasoning: m.reasoning || undefined,
  };
}

// ============================================
// State
// ============================================

interface ChatState {
  // ---- 消息 ----
  messages: ChatMessage[];
  currentStreamingMessageId: string | null;

  // ---- 流式连接 ----
  connectionStatus: ConnectionStatus;

  // ---- approve 信号 ----
  approveSignal: { version: number; payload: ApprovePayload } | null;

  // ---- 会话 ----
  sessions: SessionItem[];
  currentSessionId: string | null;
  sessionCache: Record<string, { messages: ChatMessage[] }>;
  cacheOrder: string[];
  isLoadingMessages: boolean;

  // ---- Actions ----
  addUserMessage: (content: string, imagePaths?: string[]) => void;
  startStreamingMessage: () => string;
  appendCard: (card: MessageCard) => void;
  updateLastCard: (updater: (card: MessageCard) => MessageCard) => void;
  finishStreamingMessage: (messageId: string) => void;
  setConnectionStatus: (status: ConnectionStatus) => void;
  triggerApprove: (payload: ApprovePayload) => void;
  initSession: (sessionId?: string) => string;
  clearMessages: () => void;
  loadSessions: () => Promise<void>;
  switchSession: (sessionId: string) => Promise<void>;
  deleteSession: (sessionId: string) => Promise<void>;
  /** 重置所有状态（切换账号时调用，防止旧用户数据泄露） */
  reset: () => void;
}

// ============================================
// Store
// ============================================

export const useChatStore = create<ChatState>()((set, get) => ({
  messages: [],
  currentStreamingMessageId: null,
  connectionStatus: 'disconnected',
  approveSignal: null,
  sessions: [],
  currentSessionId: null,
  sessionCache: {},
  cacheOrder: [],
  isLoadingMessages: false,

  // ============================================
  // 消息 Actions
  // ============================================

  addUserMessage: (content, imagePaths) =>
    set((s) => ({
      messages: [
        ...s.messages,
        {
          id: generateUUIDv7(),
          role: 'user',
          content,
          timestamp: Date.now(),
          cards: [],
          ...(imagePaths?.length ? { imagePaths } : {}),
        },
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
          cards: [],
        },
      ],
      currentStreamingMessageId: id,
    }));
    return id;
  },

  appendCard: (card) => {
    const sid = get().currentStreamingMessageId;
    if (!sid) return;
    useChatStore.setState((prev) => ({
      messages: prev.messages.map((m) =>
        m.id === sid ? { ...m, cards: [...m.cards, card] } : m,
      ),
    }));
  },

  updateLastCard: (updater) => {
    const sid = get().currentStreamingMessageId;
    if (!sid) return;
    useChatStore.setState((prev) => ({
      messages: prev.messages.map((m) => {
        if (m.id !== sid) return m;
        const cards = [...m.cards];
        if (cards.length === 0) return m;
        cards[cards.length - 1] = updater(cards[cards.length - 1]);
        return { ...m, cards };
      }),
    }));
  },

  finishStreamingMessage: (messageId) =>
    set((s) => ({
      messages: s.messages.map((m) =>
        m.id === messageId ? { ...m, isStreaming: false } : m,
      ),
      currentStreamingMessageId: null,
    })),

  // ============================================
  // 连接 Actions
  // ============================================

  setConnectionStatus: (status) => set({ connectionStatus: status }),

  // ============================================
  // 中断 Actions
  // ============================================

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
      sessions: s.currentSessionId
        ? s.sessions.map((x) =>
            x.id === s.currentSessionId
              ? { ...x, updatedAt: new Date().toISOString() }
              : x,
          )
        : s.sessions,
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
      set((s) => {
        const merged = s.sessions.filter((x) => {
          const rm = remote.find((r) => r.id === x.id);
          return !rm;
        });
        for (const r of remote) merged.unshift(r);
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
    // 守卫：空 sessionId 不发起请求（"新建对话"场景）
    if (!sessionId) return;
    const { sessionCache, cacheOrder } = get();
    let cachedMessages: ChatMessage[] = [];

    if (sessionCache[sessionId]) {
      cachedMessages = sessionCache[sessionId].messages;
    }

    if (cachedMessages.length === 0) {
      set({ isLoadingMessages: true });
    }

    set((s) => {
      const exists = s.sessions.some((x) => x.id === sessionId);
      const touched = [sessionId, ...s.cacheOrder.filter((id) => id !== sessionId)].slice(0, 5);
      return {
        currentSessionId: sessionId,
        messages: cachedMessages,
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
      const remoteMsgs: ChatMessage[] = (res?.messages ?? []).map((m) =>
        convertHistoryMessage(m),
      );
      const currentState = get();
      const cached = currentState.sessionCache[sessionId];
      const finalMsgs =
        cached && cached.messages.length > 0 ? cached.messages : remoteMsgs;
      set({
        messages: finalMsgs,
        isLoadingMessages: false,
        sessionCache: {
          ...currentState.sessionCache,
          [sessionId]: { messages: finalMsgs },
        },
      });
    } catch {
      set({ isLoadingMessages: false });
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

  reset: () =>
    set({
      messages: [],
      currentStreamingMessageId: null,
      connectionStatus: 'disconnected',
      approveSignal: null,
      sessions: [],
      currentSessionId: null,
      sessionCache: {},
      cacheOrder: [],
      isLoadingMessages: false,
    }),
}));
