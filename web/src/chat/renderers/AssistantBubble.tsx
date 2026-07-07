import { Avatar, Typography } from 'antd';
import { RobotOutlined } from '@ant-design/icons';
import type { MessageRendererProps } from '../types';
import { ToolCallCard } from './ToolCallCard';

/**
 * AI 助手消息渲染器
 * 注册为 role="assistant" 的渲染器，支持流式光标 + 工具调用内嵌
 */
export function AssistantBubble({ message }: MessageRendererProps) {
  return (
    <div
      style={{
        display: 'flex',
        gap: 12,
        flexDirection: 'row',
        padding: '8px 16px',
      }}
    >
      <Avatar
        icon={<RobotOutlined />}
        style={{ backgroundColor: '#52C41A', flexShrink: 0 }}
      />
      <div style={{ maxWidth: '75%' }}>
        <div
          style={{
            background: '#F5F5F5',
            borderRadius: 12,
            padding: '10px 14px',
            lineHeight: 1.6,
          }}
        >
          <Typography.Text
            style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}
          >
            {message.content}
          </Typography.Text>
          {message.isStreaming && <span className="cursor-blink" />}
        </div>

        {/* 内嵌工具调用卡片 */}
        {message.toolCalls && message.toolCalls.length > 0 && (
          <div
            style={{
              marginTop: 8,
              display: 'flex',
              flexDirection: 'column',
              gap: 6,
            }}
          >
            {message.toolCalls.map((tc) => (
              <ToolCallCard key={tc.id} call={tc} />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
