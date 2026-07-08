// ============================================
// SSE 事件类型 — v4.1 扁平化格式
//
// SSE 线格式: event: <type>\ndata: <json-payload>\n\n
// 类型由 SSE 的 event: 字段携带，data 直接是载荷 JSON，不再有 {type, data} 包装
// ============================================

export type SSEEventType =
  | 'thinking'
  | 'reasoning'
  | 'message'
  | 'tool_call'
  | 'tool_result'
  | 'interrupted'
  | 'error'
  | 'done';

export interface ThinkingData {
  text: string;
}

export interface ReasoningData {
  text: string;
  delta: boolean;
}

export interface MessageData {
  text: string;
  delta: boolean;
}

export interface ToolCallData {
  name: string;
  input: string;
}

export interface ToolResultData {
  name: string;
  output: string;
}

export interface InterruptedData {
  tool_name: string;
  reason: string;
}

export interface ErrorData {
  message: string;
  retry: boolean;
  code: string;
}
