// ============================================
// SSE 事件类型 —— 与后端 internal/domain/agent/sse.go 严格对齐
// ============================================

export type SSEEventType =
  | 'thinking'
  | 'tool_call'
  | 'tool_result'
  | 'message'
  | 'phase_change'
  | 'confirm_required'
  | 'error'
  | 'done';

export interface SSEEvent {
  type: SSEEventType;
  data: unknown;
}

export interface ThinkingData {
  message: string;
}

export interface ToolCallData {
  tool: string;
  input: unknown;
}

export interface ToolResultData {
  tool: string;
  output: unknown;
}

export interface MessageData {
  content: string;
  delta: boolean;
}

export interface PhaseChangeData {
  from: string;
  to: string;
  summary: string;
}

export interface ConfirmRequiredData {
  prompt: string;
  action: string;
  context: unknown;
}

export interface ErrorData {
  message: string;
  retry: boolean;
  code: string;
}
