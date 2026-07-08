import { useRef, useEffect } from 'react';
import { useChatStore } from '../stores/chatStore';
import { MessageRenderer } from './MessageRenderer';
import { ThinkingIndicator } from './ThinkingIndicator';

export function MessageList() {
  const messages = useChatStore((s) => s.messages);
  const reasoningContent = useChatStore((s) => s.reasoningContent);
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, reasoningContent]);

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
