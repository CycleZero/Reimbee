import { useRef, useEffect } from 'react';
import { Spin } from 'antd';
import { useChatStore } from '../stores/chatStore';
import { MessageRenderer } from './MessageRenderer';

export function MessageList() {
  const messages = useChatStore((s) => s.messages);
  const isLoading = useChatStore((s) => s.isLoadingMessages);
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  return (
    <div style={{ flex: 1, overflow: 'auto', padding: '8px 0' }}>
      {messages.length === 0 && isLoading && (
        <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', padding: 48 }}>
          <Spin tip="正在加载消息..." />
        </div>
      )}

      {messages.map((msg) => (
        <MessageRenderer key={msg.id} message={msg} />
      ))}

      <div ref={bottomRef} />
    </div>
  );
}
