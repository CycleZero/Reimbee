import { Typography } from 'antd';
import { useChatStore } from '../stores/chatStore';

export function ThinkingIndicator() {
  const isThinking = useChatStore((s) => s.isThinking);
  const thinkingMessage = useChatStore((s) => s.thinkingMessage);
  const hasStreaming = useChatStore((s) => s.currentStreamingMessageId !== null);

  if (!isThinking || hasStreaming) return null;

  return (
    <div
      style={{
        display: 'flex',
        gap: 12,
        flexDirection: 'row',
        padding: '8px 16px',
      }}
    >
      <div
        style={{
          background: '#F5F5F5',
          borderRadius: 12,
          padding: '10px 14px',
          display: 'flex',
          alignItems: 'center',
          gap: 8,
        }}
      >
        <span style={{ display: 'inline-flex', gap: 4, alignItems: 'center' }}>
          {[0, 1, 2].map((i) => (
            <span
              key={i}
              style={{
                width: 6,
                height: 6,
                borderRadius: '50%',
                background: '#1677FF',
                animation: 'thinking-bounce 1.4s infinite ease-in-out both',
                animationDelay: `${-0.32 + i * 0.16}s`,
              }}
            />
          ))}
        </span>
        <Typography.Text type="secondary" style={{ fontSize: 13 }}>
          {thinkingMessage}
        </Typography.Text>
      </div>
    </div>
  );
}
