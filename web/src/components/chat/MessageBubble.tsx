import { Avatar, Typography } from 'antd';
import { UserOutlined, RobotOutlined } from '@ant-design/icons';
import type { ChatMessage } from '@/stores/chatStore';
import { ToolCallCard } from './ToolCallCard';

interface Props {
  message: ChatMessage;
}

/**
 * 消息气泡——支持打字机流式追加
 * - user 消息：右侧对齐，蓝色头像
 * - assistant 消息：左侧对齐，AI 头像
 * - isStreaming 时追加闪烁光标
 */
export function MessageBubble({ message }: Props) {
  const isUser = message.role === 'user';

  return (
    <div
      style={{
        display: 'flex',
        gap: 12,
        flexDirection: isUser ? 'row-reverse' : 'row',
        padding: '8px 16px',
      }}
    >
      <Avatar
        icon={isUser ? <UserOutlined /> : <RobotOutlined />}
        style={{
          backgroundColor: isUser ? '#1677FF' : '#52C41A',
          flexShrink: 0,
        }}
      />
      <div style={{ maxWidth: '75%' }}>
        <div
          style={{
            background: isUser ? '#E6F4FF' : '#F5F5F5',
            borderRadius: 12,
            padding: '10px 14px',
            lineHeight: 1.6,
          }}
        >
          <Typography.Text
            style={{
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
            }}
          >
            {message.content}
          </Typography.Text>
          {message.isStreaming && <span className="cursor-blink" />}
        </div>

        {/* 内嵌工具调用卡片 */}
        {message.toolCalls && message.toolCalls.length > 0 && (
          <div style={{ marginTop: 8, display: 'flex', flexDirection: 'column', gap: 6 }}>
            {message.toolCalls.map((tc) => (
              <ToolCallCard key={tc.id} call={tc} />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
