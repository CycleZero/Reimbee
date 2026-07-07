// ============================================
// 模块级渲染器注册表
// 组件在 import 时自行注册，MessageRenderer 运行时查找
// 扩展方式：在 renderers/index.ts 中调用 registerXxxRenderer()
// ============================================

import type { MessageRendererComponent, ToolRendererComponent, MessageRole } from './types';

/** 消息角色 → 渲染组件 */
export const messageRenderers = new Map<MessageRole, MessageRendererComponent>();

/** 工具名 → 渲染组件 */
export const toolRenderers = new Map<string, ToolRendererComponent>();

/** 注册消息渲染器 */
export function registerMessageRenderer(role: MessageRole, renderer: MessageRendererComponent): void {
  messageRenderers.set(role, renderer);
}

/** 注册工具渲染器 */
export function registerToolRenderer(toolName: string, renderer: ToolRendererComponent): void {
  toolRenderers.set(toolName, renderer);
}

/** 获取消息渲染器 */
export function getMessageRenderer(role: MessageRole): MessageRendererComponent | undefined {
  return messageRenderers.get(role);
}

/** 获取工具渲染器 */
export function getToolRenderer(toolName: string): ToolRendererComponent | undefined {
  return toolRenderers.get(toolName);
}
