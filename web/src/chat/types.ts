// ============================================
// Chat 模块类型定义
// 消息、工具调用、会话、注册表等核心类型
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
  errorMessage?: string;
}

/** 中断状态 */
export type InterruptStatus = 'pending' | 'approved' | 'rejected';

/** 消息附带的中断数据（inline widget 使用） */
export interface MessageInterrupt {
  toolName: string;
  reason: string;
  status: InterruptStatus;
}

/** 卡片类型 — 对应 SSE 事件分组 */
export type CardType = 'reasoning' | 'message' | 'tool_calls' | 'interrupt';

/** 消息内的可视化卡片 — 每个卡片对应一连串同类型 SSE 事件 */
export interface MessageCard {
  type: CardType;
  /** reasoning / message 类型卡片的文本内容 */
  content?: string;
  /** tool_calls 类型卡片的工具调用列表 */
  toolCalls?: ToolCallRecord[];
  /** interrupt 类型卡片的中断数据 */
  interrupt?: MessageInterrupt;
}

/** 聊天消息 */
export interface ChatMessage {
  id: string;
  role: MessageRole;
  content: string;
  timestamp: number;
  isStreaming?: boolean;
  toolCalls?: ToolCallRecord[];
  /** 本消息对应的推理过程（模型思考链），可为空 */
  reasoning?: string;
  /** 本消息触发的中断（需要用户确认时设置） */
  interrupt?: MessageInterrupt;
  /** 卡片列表 — 流式渲染时按顺序展示的视觉卡片，可选（回退到旧布局） */
  cards?: MessageCard[];
}

/** 报销流程阶段 */
export type ReimbPhase = 'idle' | 'phase1_collect' | 'phase2_validate' | 'phase3_execute';

/** 确认提示 */
export interface ConfirmPrompt {
  action: string;
  prompt: string;
  context?: unknown;
}

/** 会话列表项 */
export interface SessionItem {
  id: string;
  title: string;
  messageCount: number;
  status: string; // active / completed / expired
  createdAt: string; // ISO 8601
  updatedAt: string; // ISO 8601
}

/** 中断提示（v4.1 Interrupt 机制） */
export interface InterruptPrompt {
  interruptId: string;
  action: string;
  context: unknown;
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

/** 消息渲染器组件 Props */
export interface MessageRendererProps {
  message: ChatMessage;
}

/** 工具渲染器组件 Props */
export interface ToolRendererProps {
  call: ToolCallRecord;
}

/** 消息渲染器组件类型 */
export type MessageRendererComponent = ComponentType<MessageRendererProps>;

/** 工具渲染器组件类型 */
export type ToolRendererComponent = ComponentType<ToolRendererProps>;

// ============================================
// SSE 事件处理器类型（用于 useChatStream 扩展）
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
