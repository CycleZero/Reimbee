// ============================================
// SSE 流式对话连接 Hook
// v4.1: 类型从 SSE event: 字段读取，data 直接是载荷 JSON
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
import type { ToolCallRecord, ChatStreamHandlers } from './types';

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080';

function getStore() {
  return useChatStore.getState();
}

export function useChatStream(
  sessionId: string | null,
  message: string | null,
  extraHandlers?: Partial<ChatStreamHandlers>,
) {
  const ctrlRef = useRef<AbortController | null>(null);
  const tokenRef = useRef(useAuthStore.getState().token);

  useEffect(() => {
    tokenRef.current = useAuthStore.getState().token;
  });

  useEffect(() => {
    if (!sessionId || !message) return;

    ctrlRef.current?.abort();
    const ctrl = new AbortController();
    ctrlRef.current = ctrl;

    const url = new URL('/api/chat/stream', BASE_URL);
    url.searchParams.set('session_id', sessionId);
    url.searchParams.set('message', message);

    getStore().setConnectionStatus('connecting');

    fetchEventSource(url.toString(), {
      method: 'GET',
      headers: { Authorization: `Bearer ${tokenRef.current}` },
      signal: ctrl.signal,
      openWhenHidden: true,

      onopen: async (response) => {
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

      onmessage(evt) {
        if (!evt.data) return;
        const payload = JSON.parse(evt.data);
        const s = getStore();

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
            if (d.delta) {
              s.appendReasoning(d.text);
            } else {
              s.setReasoning(d.text);
            }
            if (s.currentStreamingMessageId) {
              useChatStore.setState((prev) => ({
                messages: prev.messages.map((m) =>
                  m.id === prev.currentStreamingMessageId
                    ? { ...m, reasoning: (m.reasoning ?? '') + (d.delta ? d.text : '') }
                    : m,
                ),
              }));
            }
            break;
          }

          case 'tool_call': {
            const d = payload as ToolCallData;
            extraHandlers?.onToolCall?.(d.tool, d.input);
            if (s.currentStreamingMessageId) {
              s.addToolCall(s.currentStreamingMessageId, {
                id: `${d.tool}-${Date.now()}`,
                toolName: d.tool,
                status: 'running',
                input: d.input,
              } satisfies ToolCallRecord);
            }
            break;
          }

          case 'tool_result': {
            const d = payload as ToolResultData;
            extraHandlers?.onToolResult?.(d.tool, d.output);
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
            break;
          }

          case 'message': {
            const d = payload as MessageData;
            extraHandlers?.onMessage?.(d.text, d.delta);
            if (d.delta) {
              let mid = s.currentStreamingMessageId;
              if (!mid) mid = s.startStreamingMessage();
              s.appendStreamContent(mid, d.text);
            } else {
              // 完整消息：替换流式内容（校验流式结果），或创建新消息
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
            break;
          }

          case 'interrupted': {
            const d = payload as InterruptedData;
            extraHandlers?.onInterrupted?.(d.interrupt_id, d.action, d.context);
            s.setInterruptPrompt({
              interruptId: d.interrupt_id,
              action: d.action,
              context: d.context,
            });
            break;
          }

          case 'error': {
            const d = payload as ErrorData;
            extraHandlers?.onError?.(d.message, d.retry, d.code);
            if (s.currentStreamingMessageId) {
              s.appendStreamContent(s.currentStreamingMessageId, `\n\n❌ ${d.message}`);
              s.finishStreamingMessage(s.currentStreamingMessageId);
            }
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

      onerror(err) {
        getStore().setConnectionStatus('error');
        throw err;
      },
    }).catch(() => {
      getStore().setConnectionStatus('error');
    });

    return () => {
      ctrl.abort();
    };
  }, [sessionId, message]);
}
