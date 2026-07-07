import { useRef, useEffect } from 'react';
import { useChatStore } from '@/stores/chatStore';
import { PhaseIndicator } from './PhaseIndicator';
import { ThinkingDots } from './ThinkingDots';
import { MessageBubble } from './MessageBubble';
import { ConfirmModal } from './ConfirmModal';
import { ChatInput } from './ChatInput';

interface Props {
  onSend: (message: string) => void;
  disabled?: boolean;
}

export function ChatPanel({ onSend, disabled }: Props) {
  const messages = useChatStore((s) => s.messages);
  const bottomRef = useRef<HTMLDivElement>(null);

  // 自动滚动到底部
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* 阶段指示器 */}
      <div style={{ padding: '12px 16px 0' }}>
        <PhaseIndicator />
      </div>

      {/* 消息列表 */}
      <div style={{ flex: 1, overflow: 'auto', padding: '8px 0' }}>
        {messages.map((msg) => (
          <MessageBubble key={msg.id} message={msg} />
        ))}
        <ThinkingDots />
        <div ref={bottomRef} />
      </div>

      {/* 输入框 */}
      <ChatInput onSend={onSend} disabled={disabled} />

      {/* 确认弹窗 */}
      <ConfirmModal />
    </div>
  );
}
