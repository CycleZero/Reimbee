import { getMessageRenderer } from '../registry';
import type { ChatMessage } from '../types';
import { Typography } from 'antd';

interface Props {
  message: ChatMessage;
}

/**
 * 消息渲染分发器
 * 根据 message.role 从注册表中查找对应渲染组件
 * 未注册的类型使用默认纯文本渲染
 *
 * 扩展方式：调用 registerMessageRenderer('custom', MyComponent)
 */
export function MessageRenderer({ message }: Props) {
  const Renderer = getMessageRenderer(message.role);

  if (Renderer) {
    return <Renderer message={message} />;
  }

  // 默认回退：纯文本气泡
  return (
    <div style={{ padding: '8px 16px' }}>
      <div
        style={{
          background: '#F5F5F5',
          borderRadius: 12,
          padding: '10px 14px',
          display: 'inline-block',
          maxWidth: '75%',
        }}
      >
        <Typography.Text
          type="secondary"
          style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}
        >
          [{message.role}] {message.content}
        </Typography.Text>
      </div>
    </div>
  );
}
