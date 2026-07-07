import { Avatar, Typography } from 'antd';
import { UserOutlined } from '@ant-design/icons';
import type { MessageRendererProps } from '../types';

/**
 * 用户消息渲染器
 * 注册为 role="user" 的渲染器，可通过 registry 替换
 */
export function UserBubble({ message }: MessageRendererProps) {
  return (
    <div
      style={{
        display: 'flex',
        gap: 12,
        flexDirection: 'row-reverse',
        padding: '8px 16px',
      }}
    >
      <Avatar
        icon={<UserOutlined />}
        style={{ backgroundColor: '#1677FF', flexShrink: 0 }}
      />
      <div style={{ maxWidth: '75%' }}>
        <div
          style={{
            background: '#E6F4FF',
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
        </div>
      </div>
    </div>
  );
}
