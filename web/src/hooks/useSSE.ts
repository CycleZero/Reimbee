// ============================================
// SSE 连接管理 Hook
// 封装 EventSource 连接、8 种事件分发、自动重连
// ============================================

import { useEffect, useRef, useCallback } from 'react';
import { useChatStore, type ToolCallRecord } from '@/stores/chatStore';
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

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080';
const MAX_RECONNECT = 5;
const RECONNECT_BASE = 1000;

/**
 * SSE 流式对话连接 Hook
 * @param sessionId - 会话 ID（UUID v7）
 * @param message - 待发送的用户消息
 */
export function useSSE(sessionId: string | null, message: string | null) {
  const esRef = useRef<EventSource | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const retryRef = useRef(0);
  const store = useChatStore();
  const token = useAuthStore((s) => s.token);

  const handleEvent = useCallback(
    (eventType: string, rawData: string) => {
      if (!rawData) return;
      const parsed: SSEEvent = JSON.parse(rawData);

      switch (parsed.type) {
        case 'thinking': {
          store.setThinking((parsed.data as ThinkingData).message);
          break;
        }
        case 'tool_call': {
          const d = parsed.data as ToolCallData;
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
          const sid = useChatStore.getState().currentStreamingMessageId;
          if (sid) {
            const msgs = useChatStore.getState().messages;
            const msg = msgs.find((m) => m.id === sid);
            const last = msg?.toolCalls
              ?.filter((tc) => tc.toolName === d.tool && tc.status === 'running')
              .pop();
            if (last) {
              store.updateToolCall(sid, last.id, { status: 'success', output: d.output });
            }
          }
          break;
        }
        case 'message': {
          const d = parsed.data as MessageData;
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
          store.setPhase(
            d.to as 'idle' | 'phase1_collect' | 'phase2_validate' | 'phase3_execute',
          );
          break;
        }
        case 'confirm_required': {
          const d = parsed.data as ConfirmRequiredData;
          store.setConfirmPrompt({
            action: d.action,
            prompt: d.prompt,
            context: d.context,
          });
          break;
        }
        case 'error': {
          const d = parsed.data as ErrorData;
          const mid = useChatStore.getState().currentStreamingMessageId;
          if (mid) {
            store.appendStreamContent(mid, `\n\n❌ ${d.message}`);
            store.finishStreamingMessage(mid);
          }
          store.clearThinking();
          if (!d.retry) {
            esRef.current?.close();
            store.setConnectionStatus('error');
          }
          break;
        }
        case 'done': {
          const mid = useChatStore.getState().currentStreamingMessageId;
          if (mid) store.finishStreamingMessage(mid);
          store.clearThinking();
          store.setConnectionStatus('disconnected');
          esRef.current?.close();
          retryRef.current = 0;
          break;
        }
      }
    },
    [store],
  );

  useEffect(() => {
    if (!sessionId || !message || !token) return;
    retryRef.current = 0;

    const connect = () => {
      esRef.current?.close();

      const url = new URL('/api/chat/stream', BASE_URL);
      url.searchParams.set('session_id', sessionId);
      url.searchParams.set('message', message);
      url.searchParams.set('token', token);

      store.setConnectionStatus('connecting');
      const es = new EventSource(url.toString());
      esRef.current = es;

      es.onopen = () => {
        store.setConnectionStatus('connected');
        retryRef.current = 0;
      };

      const types: string[] = [
        'thinking', 'tool_call', 'tool_result', 'message',
        'phase_change', 'confirm_required', 'error', 'done',
      ];
      types.forEach((t) => {
        es.addEventListener(t, (e: MessageEvent) => handleEvent(t, e.data));
      });

      es.onerror = () => {
        store.setConnectionStatus('error');
        if (retryRef.current < MAX_RECONNECT) {
          const delay = RECONNECT_BASE * Math.pow(2, retryRef.current);
          retryRef.current++;
          timerRef.current = setTimeout(connect, delay);
        } else {
          store.clearThinking();
        }
      };
    };

    connect();

    return () => {
      esRef.current?.close();
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [sessionId, message, token, handleEvent, store]);
}
