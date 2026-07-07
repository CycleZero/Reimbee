// ============================================
// SSE 流式对话连接 Hook
// 使用 @microsoft/fetch-event-source 支持自定义 Headers（Authorization）
// 事件驱动架构——通过 handlers Map 分发，新增事件类型无需改 Hook
// ============================================

import { useEffect, useRef } from 'react';
import { fetchEventSource } from '@microsoft/fetch-event-source';
import { useChatStore } from './stores/chatStore';
import { useAuthStore } from '@/stores/authStore';
import type {
  ThinkingData,
  ToolCallData,
  ToolResultData,
  MessageData,
  PhaseChangeData,
  ConfirmRequiredData,
  ErrorData,
  SSEEvent,
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

  // 保持 token 引用最新（不触发重连）
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

      onmessage(event) {
        if (!event.data) return;
        const parsed: SSEEvent = JSON.parse(event.data);
        const s = getStore();

        switch (parsed.type) {
          case 'thinking':
            extraHandlers?.onThinking?.((parsed.data as ThinkingData).message);
            s.setThinking((parsed.data as ThinkingData).message);
            break;

          case 'tool_call': {
            const d = parsed.data as ToolCallData;
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
            const d = parsed.data as ToolResultData;
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
            const d = parsed.data as MessageData;
            extraHandlers?.onMessage?.(d.content, d.delta);
            if (d.delta) {
              let mid = s.currentStreamingMessageId;
              if (!mid) mid = s.startStreamingMessage();
              s.appendStreamContent(mid, d.content);
            } else {
              const mid = s.startStreamingMessage();
              s.appendStreamContent(mid, d.content);
              s.finishStreamingMessage(mid);
            }
            break;
          }

          case 'phase_change': {
            const d = parsed.data as PhaseChangeData;
            extraHandlers?.onPhaseChange?.(d.from, d.to, d.summary);
            s.setPhase(d.to as Parameters<typeof s.setPhase>[0]);
            break;
          }

          case 'confirm_required': {
            const d = parsed.data as ConfirmRequiredData;
            extraHandlers?.onConfirmRequired?.(d.action, d.prompt, d.context);
            s.setConfirmPrompt({ action: d.action, prompt: d.prompt, context: d.context });
            break;
          }

          case 'error': {
            const d = parsed.data as ErrorData;
            extraHandlers?.onError?.(d.message, d.retry, d.code);
            if (s.currentStreamingMessageId) {
              s.appendStreamContent(s.currentStreamingMessageId, `\n\n❌ ${d.message}`);
              s.finishStreamingMessage(s.currentStreamingMessageId);
            }
            s.clearThinking();
            s.setConnectionStatus('error');
            ctrl.abort();
            break;
          }

          case 'done':
            extraHandlers?.onDone?.();
            if (s.currentStreamingMessageId) {
              s.finishStreamingMessage(s.currentStreamingMessageId);
            }
            s.clearThinking();
            s.setConnectionStatus('disconnected');
            break;
        }
      },

      onclose() {
        getStore().setConnectionStatus('disconnected');
      },

      onerror(err) {
        getStore().setConnectionStatus('error');
        // 抛出以停止重试，单次对话失败不自动重连
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
