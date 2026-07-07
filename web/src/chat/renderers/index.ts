// ============================================
// 默认渲染器注册
// import 此文件即自动注册所有默认渲染器
// 后续可通过调用 registerXxxRenderer 覆盖/扩展
// ============================================

import { registerMessageRenderer } from '../registry';
import { UserBubble } from './UserBubble';
import { AssistantBubble } from './AssistantBubble';
import { ToolCallCard } from './ToolCallCard';

/** 注册所有默认渲染器 */
export function registerDefaultRenderers(): void {
  registerMessageRenderer('user', UserBubble);
  registerMessageRenderer('assistant', AssistantBubble);
  // 工具渲染器按需注册（ToolCallCard 在 AssistantBubble 内直接使用，作为 fallback）
}

// 模块加载时自动注册默认渲染器
registerDefaultRenderers();

/** 默认工具渲染器 */
export { ToolCallCard };
