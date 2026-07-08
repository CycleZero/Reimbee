// ============================================
// Chat 模块类型定义
// v5: 纯卡片模式 — cards[] 是消息的唯一视觉表现
//     旧字段 (content/toolCalls/reasoning/interrupt) 仅用于历史加载兼容
// ============================================

import type { ComponentType } from 'react';

/** 消息角色 */
export type MessageRole = 'user' | 'assistant' | 'system';

/** 工具调用记录 */
export interface ToolCallRecord {
  id: string;
  toolName: string;
  status: 'running' | 'success' | 'error';
  input?: unknown;
  output?: unknown;
}

/** 中断状态 */
export type InterruptStatus = 'pending' | 'approved' | 'rejected';

/** 消息附带的中断数据 */
export interface MessageInterrupt {
  toolName: string;
  reason: string;
  status: InterruptStatus;
}

/** 卡片类型 — 对应 SSE 事件组 */
export type CardType = 'thinking' | 'tool' | 'message' | 'interrupt';

/**
 * 消息卡片 — 流式渲染的视觉单元
 *
 * SSE 事件 → 卡片映射：
 *   thinking + reasoning → type='thinking' (带可折叠推理内容)
 *   tool_call + tool_result → type='tool' (每个工具一张卡片)
 *   message → type='message' (Markdown 正文)
 *   interrupted → type='interrupt' (确认/取消栏)
 */
export interface MessageCard {
  type: CardType;
  /** thinking/message 卡片文本内容 */
  content?: string;
  /** thinking 卡片的关联工具调用 */
  toolCalls?: ToolCallRecord[];
  /** tool 卡片：工具名（单工具模式，每个 tool_call 独立卡片） */
  toolName?: string;
  /** tool 卡片：执行状态 */
  status?: 'running' | 'success' | 'error';
  /** tool 卡片：输入参数 */
  input?: unknown;
  /** tool 卡片：输出结果 */
  output?: unknown;
  /** interrupt 卡片：中断数据 */
  interrupt?: MessageInterrupt;
  /** thinking 卡片：状态文字（如 "正在处理..."） */
  thinkingText?: string;
}

/** 聊天消息 */
export interface ChatMessage {
  id: string;
  role: MessageRole;
  /** 用于历史消息加载时的纯文本内容，渲染以 cards 为准 */
  content: string;
  timestamp: number;
  isStreaming?: boolean;
  /** 卡片列表 — 流式渲染的唯一视觉来源 */
  cards: MessageCard[];
  // ---- 以下字段仅用于历史消息加载兼容，渲染不使用 ----
  toolCalls?: ToolCallRecord[];
  reasoning?: string;
  interrupt?: MessageInterrupt;
}

/** 会话列表项 */
export interface SessionItem {
  id: string;
  title: string;
  messageCount: number;
  status: string;
  createdAt: string;
  updatedAt: string;
}

/** approve 请求体 */
export interface ApprovePayload {
  session_id: string;
  approved: boolean;
  reason: string;
}

/** 连接状态 */
export type ConnectionStatus = 'disconnected' | 'connecting' | 'connected' | 'error';

// ============================================
// 注册表组件类型
// ============================================

export interface MessageRendererProps {
  message: ChatMessage;
}

export interface ToolRendererProps {
  call: ToolCallRecord;
}

export type MessageRendererComponent = ComponentType<MessageRendererProps>;
export type ToolRendererComponent = ComponentType<ToolRendererProps>;

// ============================================
// SSE 事件处理器类型
// ============================================

export interface ChatStreamHandlers {
  onThinking?: (text: string) => void;
  onReasoning?: (text: string, delta: boolean) => void;
  onToolCall?: (tool: string, input: unknown) => void;
  onToolResult?: (tool: string, output: unknown) => void;
  onMessage?: (text: string, delta: boolean) => void;
  onInterrupted?: (toolName: string, reason: string) => void;
  onError?: (message: string, retry: boolean, code: string) => void;
  onDone?: () => void;
}
