// ============================================
// SSE 流式对话连接 Hook
// 使用 @microsoft/fetch-event-source 支持自定义 Headers（Authorization）
// 事件驱动架构——通过 handlers Map 分发，新增事件类型无需改 Hook
// ============================================

import { useEffect, useRef, useCallback } from 'react';
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

/**
 * SSE 流式对话连接 Hook
 *
 * @param sessionId - 会话 ID
 * @param message - 待发送的用户消息（null 时不连接）
 * @param extraHandlers - 扩展事件处理器（用于外部注入自定义事件处理）
 */
export function useChatStream(
  sessionId: string | null,
  message: string | null,
  extraHandlers?: Partial<ChatStreamHandlers>,
) {
  const ctrlRef = useRef<AbortController | null>(null);
  const store = useChatStore();
  const token = useAuthStore((s) => s.token);

  // 解析 SSE 事件并分发到 store / extraHandlers
  const handleEvent = useCallback(
    (eventType: string, rawData: string) => {
      if (!rawData) return;
      const parsed: SSEEvent = JSON.parse(rawData);

      switch (parsed.type) {
        case 'thinking': {
          const msg = (parsed.data as ThinkingData).message;
          extraHandlers?.onThinking?.(msg);
          store.setThinking(msg);
          break;
        }
        case 'tool_call': {
          const d = parsed.data as ToolCallData;
          extraHandlers?.onToolCall?.(d.tool, d.input);
          const sid = useChatStore.getState().currentStreamingMessageId;
          if (sid) {
            store.addToolCall(sid, {
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
          const sid = useChatStore.getState().currentStreamingMessageId;
          if (sid) {
            const msgs = useChatStore.getState().messages;
            const msg = msgs.find((m) => m.id === sid);
            const last = msg?.toolCalls
              ?.filter((tc) => tc.toolName === d.tool && tc.status === 'running')
              .pop();
            if (last) {
              store.updateToolCall(sid, last.id, {
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
            let mid = useChatStore.getState().currentStreamingMessageId;
            if (!mid) mid = store.startStreamingMessage();
            store.appendStreamContent(mid, d.content);
          } else {
            const mid = store.startStreamingMessage();
            store.appendStreamContent(mid, d.content);
            store.finishStreamingMessage(mid);
          }
          break;
        }
        case 'phase_change': {
          const d = parsed.data as PhaseChangeData;
          extraHandlers?.onPhaseChange?.(d.from, d.to, d.summary);
          store.setPhase(
            d.to as 'idle' | 'phase1_collect' | 'phase2_validate' | 'phase3_execute',
          );
          break;
        }
        case 'confirm_required': {
          const d = parsed.data as ConfirmRequiredData;
          extraHandlers?.onConfirmRequired?.(d.action, d.prompt, d.context);
          store.setConfirmPrompt({
            action: d.action,
            prompt: d.prompt,
            context: d.context,
          });
          break;
        }
        case 'error': {
          const d = parsed.data as ErrorData;
          extraHandlers?.onError?.(d.message, d.retry, d.code);
          const mid = useChatStore.getState().currentStreamingMessageId;
          if (mid) {
            store.appendStreamContent(mid, `\n\n❌ ${d.message}`);
            store.finishStreamingMessage(mid);
          }
          store.clearThinking();
          if (!d.retry) {
            ctrlRef.current?.abort();
            store.setConnectionStatus('error');
          }
          break;
        }
        case 'done': {
          extraHandlers?.onDone?.();
          const mid = useChatStore.getState().currentStreamingMessageId;
          if (mid) store.finishStreamingMessage(mid);
          store.clearThinking();
          store.setConnectionStatus('disconnected');
          break;
        }
      }
    },
    [store, extraHandlers],
  );

  useEffect(() => {
    if (!sessionId || !message || !token) return;

    // 取消上一次连接
    ctrlRef.current?.abort();
    const ctrl = new AbortController();
    ctrlRef.current = ctrl;

    const url = new URL('/api/chat/stream', BASE_URL);
    url.searchParams.set('session_id', sessionId);
    url.searchParams.set('message', message);

    store.setConnectionStatus('connecting');

    fetchEventSource(url.toString(), {
      method: 'GET',
      headers: {
        Authorization: `Bearer ${token}`,
      },
      signal: ctrl.signal,
      onopen: async (response) => {
        if (response.ok) {
          store.setConnectionStatus('connected');
          return;
        }
        // 401：token 过期
        if (response.status === 401) {
          useAuthStore.getState().logout();
          window.location.href = '/login';
        }
        throw new Error(`SSE 连接失败 (${response.status})`);
      },
      onmessage: (event) => {
        handleEvent(event.event, event.data);
      },
      onclose: () => {
        store.setConnectionStatus('disconnected');
      },
      onerror: (err) => {
        store.setConnectionStatus('error');
        // 不抛出则 fetchEventSource 自动重试（最长 5 次，每次退避翻倍）
        // 如果 AbortController 已取消则不重试
        if (ctrl.signal.aborted) throw err;
      },
    }).catch(() => {
      // 连接被取消或网络错误，静默处理
      store.clearThinking();
    });

    return () => {
      ctrl.abort();
    };
  }, [sessionId, message, token, handleEvent, store]);
}
