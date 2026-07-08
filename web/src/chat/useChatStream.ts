// ============================================
// SSE 流式对话连接 Hook
// v4.1: 类型从 SSE event: 字段读取，data 直接是载荷 JSON
// v4.2: 支持 approve 模式 — POST /api/chat/approve 恢复中断执行
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
import type { ToolCallRecord, MessageCard, ChatStreamHandlers, ApprovePayload } from './types';

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080';

function getStore() {
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
  const currentCardTypeRef = useRef<MessageCard['type'] | null>(null);

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

    getStore().setConnectionStatus('connecting');

    const sharedHandlers = {
      onopen: async (response: Response) => {
        if (response.ok) {
          getStore().setConnectionStatus('connected');
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
        const s = getStore();

        // 延迟创建流式消息（首个需要卡片的 SSE 事件触发）
        function ensureStreamingMessage(): void {
          if (!getStore().currentStreamingMessageId) {
            getStore().startStreamingMessage();
          }
        }

        // 确保最后一个卡片是目标类型，不是则推入新卡片
        function ensureCard(cardType: MessageCard['type']): void {
          ensureStreamingMessage();
          getStore().updateStreamingCards((cards) => {
            const last = cards[cards.length - 1];
            if (last?.type === cardType) return cards;
            return [...cards, { type: cardType }];
          });
          currentCardTypeRef.current = cardType;
        }

        // delta=true: 追加内容到末尾卡片；delta=false: 替换末尾匹配类型卡片内容
        function appendCardContent(cardType: MessageCard['type'], text: string, delta: boolean): void {
          getStore().updateStreamingCards((cards) => {
            const updated = [...cards];
            const last = updated[updated.length - 1];
            if (delta) {
              if (last) {
                updated[updated.length - 1] = { ...last, content: (last.content ?? '') + text };
              }
            } else {
              // delta=false: 向前搜索最后一张匹配类型的卡片替换（而非只看末尾）
              let lastMatchIdx = -1;
              for (let i = updated.length - 1; i >= 0; i--) {
                if (updated[i].type === cardType) {
                  lastMatchIdx = i;
                  break;
                }
              }
              if (lastMatchIdx >= 0) {
                updated[lastMatchIdx] = { ...updated[lastMatchIdx], content: text };
              } else {
                updated.push({ type: cardType, content: text });
              }
            }
            return updated;
          });
        }

        // 向当前推理卡片追加工具调用，或创建独立的 tool_calls 卡片
        function addToolCallToCard(tool: string, input: unknown): void {
          const callRecord: ToolCallRecord = {
            id: `${tool}-${Date.now()}`,
            toolName: tool,
            status: 'running',
            input,
          };
          getStore().updateStreamingCards((cards) => {
            const updated = [...cards];
            const last = updated[updated.length - 1];
            if (last && last.type === 'reasoning') {
              updated[updated.length - 1] = {
                ...last,
                toolCalls: [...(last.toolCalls ?? []), callRecord],
              };
            } else if (last?.type === 'tool_calls') {
              updated[updated.length - 1] = {
                ...last,
                toolCalls: [...(last.toolCalls ?? []), callRecord],
              };
            } else {
              updated.push({ type: 'tool_calls', toolCalls: [callRecord] });
              currentCardTypeRef.current = 'tool_calls';
            }
            return updated;
          });
        }

        // 在所有卡片中查找匹配工具调用并更新状态
        function updateToolCallInCards(tool: string, output: unknown): void {
          getStore().updateStreamingCards((cards) =>
            cards.map((card) => {
              if (!card.toolCalls) return card;
              return {
                ...card,
                toolCalls: card.toolCalls.map((tc) =>
                  tc.toolName === tool && tc.status === 'running'
                    ? { ...tc, status: 'success' as const, output }
                    : tc,
                ),
              };
            }),
          );
        }

        switch (evt.event) {
          case 'thinking': {
            const d = payload as ThinkingData;
            extraHandlers?.onThinking?.(d.text);
            s.setThinking(d.text);
            break;
          }

          case 'reasoning': {
            const d = payload as ReasoningData;
            extraHandlers?.onReasoning?.(d.text, d.delta);
            // 旧行为：更新 store 推理状态
            if (d.delta) {
              s.appendReasoning(d.text);
            } else {
              s.setReasoning(d.text);
            }
            // 旧行为：更新消息的 reasoning 字段
            if (s.currentStreamingMessageId) {
              useChatStore.setState((prev) => ({
                messages: prev.messages.map((m) =>
                  m.id === prev.currentStreamingMessageId
                    ? { ...m, reasoning: (m.reasoning ?? '') + (d.delta ? d.text : '') }
                    : m,
                ),
              }));
            }
            // 新行为：卡片流式
            if (d.delta) {
              if (currentCardTypeRef.current !== 'reasoning') {
                ensureCard('reasoning');
              }
              appendCardContent('reasoning', d.text, true);
            } else {
              appendCardContent('reasoning', d.text, false);
            }
            break;
          }

          case 'tool_call': {
            const d = payload as ToolCallData;
            // 过滤空工具名
            if (!d.tool) break;
            extraHandlers?.onToolCall?.(d.tool, d.input);
            // 旧行为：追加工具调用到消息
            if (s.currentStreamingMessageId) {
              s.addToolCall(s.currentStreamingMessageId, {
                id: `${d.tool}-${Date.now()}`,
                toolName: d.tool,
                status: 'running',
                input: d.input,
              } satisfies ToolCallRecord);
            }
            // 新行为：卡片流式
            ensureStreamingMessage();
            addToolCallToCard(d.tool, d.input);
            break;
          }

          case 'tool_result': {
            const d = payload as ToolResultData;
            extraHandlers?.onToolResult?.(d.tool, d.output);
            // 旧行为：更新工具调用状态
            if (s.currentStreamingMessageId) {
              const msg = s.messages.find((m) => m.id === s.currentStreamingMessageId);
              const last = msg?.toolCalls
                ?.filter((tc) => tc.toolName === d.tool && tc.status === 'running')
                .pop();
              if (last) {
                s.updateToolCall(s.currentStreamingMessageId, last.id, {
                  status: 'success',
                  output: d.output,
                });
              }
            }
            // 新行为：更新卡片中工具调用状态
            updateToolCallInCards(d.tool, d.output);
            break;
          }

          case 'message': {
            const d = payload as MessageData;
            extraHandlers?.onMessage?.(d.text, d.delta);
            // 旧行为：追加/替换消息内容
            if (d.delta) {
              let mid = s.currentStreamingMessageId;
              if (!mid) mid = s.startStreamingMessage();
              s.appendStreamContent(mid, d.text);
            } else {
              if (s.currentStreamingMessageId) {
                useChatStore.setState((prev) => ({
                  messages: prev.messages.map((m) =>
                    m.id === prev.currentStreamingMessageId
                      ? { ...m, content: d.text, isStreaming: false }
                      : m,
                  ),
                  currentStreamingMessageId: null,
                }));
              } else {
                const mid = s.startStreamingMessage();
                s.appendStreamContent(mid, d.text);
                s.finishStreamingMessage(mid);
              }
            }
            // 新行为：卡片流式
            if (d.delta) {
              if (currentCardTypeRef.current !== 'message') {
                ensureCard('message');
              }
              appendCardContent('message', d.text, true);
            } else {
              appendCardContent('message', d.text, false);
            }
            break;
          }

          case 'interrupted': {
            const d = payload as InterruptedData;
            extraHandlers?.onInterrupted?.(d.tool_name, d.reason);
            // 推入中断卡片
            getStore().updateStreamingCards((cards) => [
              ...cards,
              {
                type: 'interrupt' as const,
                interrupt: {
                  toolName: d.tool_name,
                  reason: d.reason,
                  status: 'pending' as const,
                },
              },
            ]);
            currentCardTypeRef.current = null;
            if (s.currentStreamingMessageId) {
              s.finishStreamingMessage(s.currentStreamingMessageId);
            }
            // 旧行为：将中断挂到最近一条 assistant 消息上
            useChatStore.setState((prev) => {
              const lastAssistant = [...prev.messages].reverse().find((m) => m.role === 'assistant');
              if (!lastAssistant) return {};
              return {
                messages: prev.messages.map((m) =>
                  m.id === lastAssistant.id
                    ? {
                        ...m,
                        interrupt: {
                          toolName: d.tool_name,
                          reason: d.reason,
                          status: 'pending' as const,
                        },
                      }
                    : m,
                ),
              };
            });
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
            s.clearThinking();
            s.clearReasoning();
            s.setConnectionStatus('disconnected');
            break;
          }

          case 'error': {
            const d = payload as ErrorData;
            extraHandlers?.onError?.(d.message, d.retry, d.code);
            if (s.currentStreamingMessageId) {
              s.appendStreamContent(s.currentStreamingMessageId, `\n\n❌ ${d.message}`);
              s.finishStreamingMessage(s.currentStreamingMessageId);
            }
            currentCardTypeRef.current = null;
            s.clearThinking();
            s.clearReasoning();
            s.setConnectionStatus('error');
            ctrl.abort();
            break;
          }

          case 'done': {
            extraHandlers?.onDone?.();
            if (s.currentStreamingMessageId) {
              s.finishStreamingMessage(s.currentStreamingMessageId);
            }
            currentCardTypeRef.current = null;
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
            s.clearThinking();
            s.clearReasoning();
            s.setConnectionStatus('disconnected');
            break;
          }
        }
      },

      onclose() {
        getStore().setConnectionStatus('disconnected');
      },

      onerror(err: unknown) {
        getStore().setConnectionStatus('error');
        throw err;
      },
    };

    if (isApprove) {
      const url = `${BASE_URL}/api/chat/approve`;
      fetchEventSource(url, {
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
        getStore().setConnectionStatus('error');
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
        getStore().setConnectionStatus('error');
      });
    }

    return () => {
      ctrl.abort();
    };
  }, [sessionId, message, approve?.version]);
}
