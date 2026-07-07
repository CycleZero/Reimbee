import type { ReactNode } from 'react';
import { MessageList } from './components/MessageList';
import { ChatInput } from './components/ChatInput';
import { ConfirmModal } from './components/ConfirmModal';
import './renderers/index'; // 触发默认渲染器注册

interface Props {
  /** 消息区域上方插槽（如 PhaseIndicator） */
  header?: ReactNode;
  /** 输入区域上方插槽 */
  inputPrefix?: ReactNode;
  /** 发送按钮右侧插槽 */
  inputSuffix?: ReactNode;
  /** 输入框占位文字 */
  placeholder?: string;
  /** 发送回调 */
  onSend: (message: string) => void;
  /** 是否禁用输入 */
  disabled?: boolean;
}

/**
 * 可组合聊天布局
 *
 * 扩展方式：
 * - header: 注入阶段指示器、会话标题等
 * - inputPrefix: 注入附件按钮、快捷回复等
 * - inputSuffix: 注入语音输入、更多操作等
 * - children: 替换整个消息区域（默认使用 MessageList）
 */
export function ChatLayout({
  header,
  inputPrefix,
  inputSuffix,
  placeholder,
  onSend,
  disabled,
}: Props) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* 头部插槽 */}
      {header && <div style={{ padding: '12px 16px 0' }}>{header}</div>}

      {/* 消息列表 */}
      <MessageList />

      {/* 输入区域 */}
      <ChatInput
        onSend={onSend}
        disabled={disabled}
        placeholder={placeholder}
        prefix={inputPrefix}
        suffix={inputSuffix}
      />

      {/* 确认弹窗 */}
      <ConfirmModal />
    </div>
  );
}
