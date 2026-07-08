import { Avatar } from 'antd';
import { UserOutlined } from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { MessageRendererProps } from '../types';

const markdownStyle: React.CSSProperties = {
  lineHeight: 1.7,
  wordBreak: 'break-word',
};

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
          }}
        >
          <ReactMarkdown remarkPlugins={[remarkGfm]} components={{
            p: ({ children }) => <p style={markdownStyle}>{children}</p>,
            code: ({ className, children, ...props }) => {
              const isInline = !className;
              return isInline ? (
                <code style={{ background: '#d6e8ff', padding: '2px 6px', borderRadius: 4, fontSize: '0.9em' }} {...props}>
                  {children}
                </code>
              ) : (
                <pre style={{ background: '#1e1e1e', color: '#d4d4d4', padding: 12, borderRadius: 8, overflow: 'auto', fontSize: '0.85em' }}>
                  <code className={className} {...props}>{children}</code>
                </pre>
              );
            },
          }}>
            {message.content}
          </ReactMarkdown>
        </div>
      </div>
    </div>
  );
}
