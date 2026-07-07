import { useRef, useEffect } from 'react';
import { useChatStore } from '../stores/chatStore';
import { MessageRenderer } from './MessageRenderer';
import { ThinkingIndicator } from './ThinkingIndicator';

/**
 * 消息列表
 * 遍历 messages 并使用 MessageRenderer 分发渲染
 * 自动滚动到底部
 */
export function MessageList() {
  const messages = useChatStore((s) => s.messages);
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  return (
    <div style={{ flex: 1, overflow: 'auto', padding: '8px 0' }}>
      {messages.map((msg) => (
        <MessageRenderer key={msg.id} message={msg} />
      ))}
      <ThinkingIndicator />
      <div ref={bottomRef} />
    </div>
  );
}
