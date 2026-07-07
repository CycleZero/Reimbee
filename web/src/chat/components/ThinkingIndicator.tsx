import { Typography } from 'antd';
import { useChatStore } from '../stores/chatStore';

/**
 * AI 思考中指示器
 * 显示跳动圆点 + 状态文字
 */
export function ThinkingIndicator() {
  const { isThinking, thinkingMessage } = useChatStore();
  if (!isThinking) return null;

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        padding: '4px 16px',
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
      <Typography.Text type="secondary">{thinkingMessage}</Typography.Text>
    </div>
  );
}
