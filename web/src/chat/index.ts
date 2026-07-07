// ============================================
// Chat 模块公共导出
// ============================================

// 布局
export { ChatLayout } from './ChatLayout';

// 注册表（供外部扩展）
export {
  registerMessageRenderer,
  registerToolRenderer,
  getMessageRenderer,
  getToolRenderer,
  messageRenderers,
  toolRenderers,
} from './registry';

// 渲染器注册（页面级 import 触发）
export { registerDefaultRenderers } from './renderers/index';

// Store
export { useChatStore } from './stores/chatStore';

// Hook
export { useChatStream } from './useChatStream';

// 类型
export type {
  ChatMessage,
  ToolCallRecord,
  ConfirmPrompt,
  ReimbPhase,
  SessionItem,
  ConnectionStatus,
  MessageRole,
  MessageRendererProps,
  ToolRendererProps,
  MessageRendererComponent,
  ToolRendererComponent,
  ChatStreamHandlers,
} from './types';

// 组件（按需导入）
export { MessageList } from './components/MessageList';
export { MessageRenderer } from './components/MessageRenderer';
export { ChatInput } from './components/ChatInput';
export { ThinkingIndicator } from './components/ThinkingIndicator';
export { PhaseIndicator } from './components/PhaseIndicator';
export { ConfirmModal } from './components/ConfirmModal';
