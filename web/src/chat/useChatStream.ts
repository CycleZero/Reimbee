// ============================================
// SSE 流式对话连接 Hook (v5 纯卡片模式)
//
// SSE 事件 → 卡片映射：
//   thinking  → 创建/更新 thinking 卡片
//   reasoning → 追加推理内容到 thinking 卡片
//   tool_call → 为每个工具创建独立卡片
//   tool_result→ 更新对应工具卡片状态
//   message   → 创建/追加 message 卡片
//   interrupted→ 创建 interrupt 卡片
//   error     → 追加错误文本
//   done      → 完成并缓存
//
// 类型切换时自动弹新卡片（reasoning→tool→message 各有独立卡片）
// ============================================

import { useEffect, useRef } from 'react';
import { fetchEventSource } from '@microsoft/fetch-event-source';
import { useChatStore } from './stores/chatStore';
import { useAuthStore } from '@/stores/authStore';
import type {
  ThinkingData,
  ReasoningData,
  ToolCallData,
  ToolResultData,
  MessageData,
  InterruptedData,
  ErrorData,
} from '@/types/sse';
import type { ApprovePayload, ChatStreamHandlers } from './types';

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080';

function store() {
  return useChatStore.getState();
}

interface ApproveTrigger {
  payload: ApprovePayload;
  version: number;
}

export function useChatStream(
  sessionId: string | null,
  message: string | null,
  extraHandlers?: Partial<ChatStreamHandlers>,
  approve?: ApproveTrigger | null,
) {
  const ctrlRef = useRef<AbortController | null>(null);
  const tokenRef = useRef(useAuthStore.getState().token);

  useEffect(() => {
    tokenRef.current = useAuthStore.getState().token;
  });

  useEffect(() => {
    const isApprove = !!approve;
    const hasMessage = !!sessionId && !!message;

    if (!isApprove && !hasMessage) return;

    ctrlRef.current?.abort();
    const ctrl = new AbortController();
    ctrlRef.current = ctrl;

    store().setConnectionStatus('connecting');

    const sharedHandlers = {
      onopen: async (response: Response) => {
        if (response.ok) {
          store().setConnectionStatus('connected');
          return;
        }
        if (response.status === 401) {
          useAuthStore.getState().logout();
          window.location.href = '/login';
        }
        throw new Error(`SSE 连接失败 (${response.status})`);
      },

      onmessage(evt: { event: string; data: string }) {
        if (!evt.data) return;
        const payload = JSON.parse(evt.data);
        const s = store();

        // 延迟创建流式消息（首个需要卡片的 SSE 事件触发）
        const ensureStreaming = (): void => {
          if (!store().currentStreamingMessageId) {
            store().startStreamingMessage();
          }
        };

        // 获取最后一张卡片的类型（用于判断是否需要弹新卡片）
        const lastCardType = (): string | undefined => {
          const mid = store().currentStreamingMessageId;
          if (!mid) return undefined;
          const msg = store().messages.find((m) => m.id === mid);
          return msg?.cards?.[msg.cards.length - 1]?.type;
        };

        switch (evt.event) {

          // ============================================
          // thinking — 创建 thinking 卡片，显示 "正在处理..."
          // ============================================
          case 'thinking': {
            const d = payload as ThinkingData;
            extraHandlers?.onThinking?.(d.text);
            ensureStreaming();
            // 如果最后一张不是 thinking，弹新卡片；否则更新文字
            if (lastCardType() !== 'thinking') {
              s.appendCard({ type: 'thinking', thinkingText: d.text });
            } else {
              s.updateLastCard((card) => ({ ...card, thinkingText: d.text }));
            }
            break;
          }

          // ============================================
          // reasoning — 仅处理 delta=true 分片，完整消息忽略
          // ============================================
          case 'reasoning': {
            const d = payload as ReasoningData;
            if (!d.delta) break; // 完整消息直接忽略（内容已通过分片送达）
            extraHandlers?.onReasoning?.(d.text, true);
            ensureStreaming();
            if (lastCardType() !== 'thinking') {
              s.appendCard({
                type: 'thinking',
                content: d.text,
                thinkingText: '思考中...',
              });
            } else {
              s.updateLastCard((card) => ({
                ...card,
                content: (card.content ?? '') + d.text,
              }));
            }
            break;
          }

          // ============================================
          // tool_call — 每个工具独立一张卡片，默认折叠
          // ============================================
          case 'tool_call': {
            const d = payload as ToolCallData;
            if (!d.name) break;
            extraHandlers?.onToolCall?.(d.name, d.input);
            ensureStreaming();
            s.appendCard({
              type: 'tool',
              toolName: d.name,
              status: 'running',
              input: d.input,
            });
            break;
          }

          // ============================================
          // tool_result — 更新最后一张匹配的工具卡片
          // ============================================
          case 'tool_result': {
            const d = payload as ToolResultData;
            extraHandlers?.onToolResult?.(d.name, d.output);
            const mid = store().currentStreamingMessageId;
            if (!mid) break;
            useChatStore.setState((prev) => ({
              messages: prev.messages.map((m) => {
                if (m.id !== mid) return m;
                const cards = [...m.cards];
                for (let i = cards.length - 1; i >= 0; i--) {
                  if (
                    cards[i].type === 'tool' &&
                    cards[i].toolName === d.name &&
                    cards[i].status === 'running'
                  ) {
                    cards[i] = {
                      ...cards[i],
                      status: 'success',
                      output: d.output,
                    };
                    break;
                  }
                }
                return { ...m, cards };
              }),
            }));
            break;
          }

          // ============================================
          // message — 仅处理 delta=true 分片，完整消息忽略
          // ============================================
          case 'message': {
            const d = payload as MessageData;
            if (!d.delta) break; // 完整消息直接忽略（内容已通过分片送达）
            extraHandlers?.onMessage?.(d.text, true);
            ensureStreaming();
            if (lastCardType() !== 'message') {
              s.appendCard({ type: 'message', content: d.text });
            } else {
              s.updateLastCard((card) => ({
                ...card,
                content: (card.content ?? '') + d.text,
              }));
            }
            break;
          }

          // ============================================
          // interrupted — 中断卡片，确认/取消按钮
          // ============================================
          case 'interrupted': {
            const d = payload as InterruptedData;
            extraHandlers?.onInterrupted?.(d.tool_name, d.reason);
            ensureStreaming();
            s.appendCard({
              type: 'interrupt',
              interrupt: {
                toolName: d.tool_name,
                reason: d.reason,
                status: 'pending',
              },
            });
            // 完成当前流式消息
            if (s.currentStreamingMessageId) {
              s.finishStreamingMessage(s.currentStreamingMessageId);
            }
            // 缓存
            const sid = s.currentSessionId;
            if (sid && s.messages.length > 0) {
              useChatStore.setState((prev) => {
                const touched = [sid, ...prev.cacheOrder.filter((id) => id !== sid)].slice(0, 5);
                return {
                  sessionCache: {
                    ...prev.sessionCache,
                    [sid]: { messages: prev.messages },
                  },
                  cacheOrder: touched,
                };
              });
            }
            s.setConnectionStatus('disconnected');
            break;
          }

          // ============================================
          // error — 错误提示
          // ============================================
          case 'error': {
            const d = payload as ErrorData;
            extraHandlers?.onError?.(d.message, d.retry, d.code);
            ensureStreaming();
            // 追加错误信息到当前最后一张 message 卡片，或创建新的 message 卡片
            if (lastCardType() === 'message') {
              s.updateLastCard((card) => ({
                ...card,
                content: (card.content ?? '') + `\n\n> ⚠️ ${d.message}`,
              }));
            } else {
              s.appendCard({ type: 'message', content: `⚠️ ${d.message}` });
            }
            if (s.currentStreamingMessageId) {
              s.finishStreamingMessage(s.currentStreamingMessageId);
            }
            s.setConnectionStatus('error');
            ctrl.abort();
            break;
          }

          // ============================================
          // done — 完成本轮
          // ============================================
          case 'done': {
            extraHandlers?.onDone?.();
            if (s.currentStreamingMessageId) {
              s.finishStreamingMessage(s.currentStreamingMessageId);
            }
            const sid = s.currentSessionId;
            if (sid && s.messages.length > 0) {
              useChatStore.setState((prev) => {
                const touched = [sid, ...prev.cacheOrder.filter((id) => id !== sid)].slice(0, 5);
                return {
                  sessionCache: {
                    ...prev.sessionCache,
                    [sid]: { messages: prev.messages },
                  },
                  cacheOrder: touched,
                };
              });
            }
            s.setConnectionStatus('disconnected');
            break;
          }
        }
      },

      onclose() {
        store().setConnectionStatus('disconnected');
      },

      onerror(err: unknown) {
        store().setConnectionStatus('error');
        throw err;
      },
    };

    if (isApprove) {
      fetchEventSource(`${BASE_URL}/api/chat/approve`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${tokenRef.current}`,
        },
        body: JSON.stringify(approve.payload),
        signal: ctrl.signal,
        openWhenHidden: true,
        ...sharedHandlers,
      }).catch(() => {
        store().setConnectionStatus('error');
      });
    } else {
      const url = new URL('/api/chat/stream', BASE_URL);
      url.searchParams.set('session_id', sessionId!);
      url.searchParams.set('message', message!);

      fetchEventSource(url.toString(), {
        method: 'GET',
        headers: { Authorization: `Bearer ${tokenRef.current}` },
        signal: ctrl.signal,
        openWhenHidden: true,
        ...sharedHandlers,
      }).catch(() => {
        store().setConnectionStatus('error');
      });
    }

    return () => {
      ctrl.abort();
    };
  }, [sessionId, message, approve?.version]);
}
