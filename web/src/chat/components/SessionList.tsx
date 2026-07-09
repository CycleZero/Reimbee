import { useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { Button, Popconfirm, Typography, message } from 'antd';
import {
  PlusOutlined,
  DeleteOutlined,
  MessageOutlined,
} from '@ant-design/icons';
import { useChatStore } from '../stores/chatStore';

const { Text, Paragraph } = Typography;
const SIDEBAR_WIDTH = 260;

function formatRelativeTime(isoString: string): string {
  const now = Date.now();
  const then = new Date(isoString).getTime();
  const diff = now - then;
  const minutes = Math.floor(diff / 60000);
  if (minutes < 1) return '刚刚';
  if (minutes < 60) return `${minutes}分钟前`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}小时前`;
  const days = Math.floor(hours / 24);
  if (days < 7) return `${days}天前`;
  return new Date(isoString).toLocaleDateString('zh-CN');
}

export function SessionList() {
  const navigate = useNavigate();
  const sessions = useChatStore((s) => s.sessions);
  const currentId = useChatStore((s) => s.currentSessionId);
  const switchSession = useChatStore((s) => s.switchSession);
  const deleteSession = useChatStore((s) => s.deleteSession);
  const newSession = useChatStore((s) => s.newSession);

  const handleSelect = useCallback(
    (id: string) => {
      switchSession(id);
      navigate(`/chat/${id}`, { replace: true });
    },
    [switchSession, navigate],
  );

  const handleDelete = useCallback(
    async (id: string) => {
      await deleteSession(id);
      if (currentId === id) {
        message.info('已删除当前会话');
      }
    },
    [deleteSession, currentId],
  );

  return (
    <div
      style={{
        width: SIDEBAR_WIDTH,
        minWidth: SIDEBAR_WIDTH,
        display: 'flex',
        flexDirection: 'column',
        borderRight: '1px solid #f0f0f0',
        background: '#fafafa',
        overflow: 'hidden',
      }}
    >
      <div style={{ padding: '12px' }}>
        <Button
          type="primary"
          block
          icon={<PlusOutlined />}
          onClick={() => {
            newSession();
            navigate('/chat', { replace: true });
          }}
        >
          新建对话
        </Button>
      </div>

      <div style={{ height: 1, background: '#f0f0f0', margin: '0 12px' }} />

      <div style={{ flex: 1, overflowY: 'auto', padding: '4px 0' }}>
        {sessions.length === 0 && (
          <div style={{ padding: '32px 16px', textAlign: 'center' }}>
            <Text type="secondary">暂无对话记录</Text>
          </div>
        )}

        {sessions.map((item) => {
          const isActive = item.id === currentId;
          return (
            <div
              key={item.id}
              onClick={() => handleSelect(item.id)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') handleSelect(item.id);
              }}
              role="button"
              tabIndex={0}
              style={{
                padding: '10px 12px',
                margin: '2px 8px',
                borderRadius: 8,
                cursor: 'pointer',
                background: isActive ? '#e6f4ff' : 'transparent',
                transition: 'background 0.15s',
                display: 'flex',
                alignItems: 'flex-start',
                gap: 8,
              }}
              onMouseEnter={(e) => {
                if (!isActive)
                  (e.currentTarget as HTMLElement).style.background = '#f5f5f5';
              }}
              onMouseLeave={(e) => {
                if (!isActive)
                  (e.currentTarget as HTMLElement).style.background = 'transparent';
              }}
            >
              <MessageOutlined
                style={{
                  marginTop: 3,
                  color: isActive ? '#1677ff' : '#999',
                  fontSize: 14,
                }}
              />
              <div style={{ flex: 1, minWidth: 0 }}>
                <Paragraph
                  ellipsis={{ rows: 1 }}
                  style={{
                    margin: 0,
                    fontSize: 13,
                    fontWeight: isActive ? 600 : 400,
                    color: isActive ? '#1677ff' : '#333',
                  }}
                >
                  {item.title}
                </Paragraph>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 2 }}>
                  <Text type="secondary" style={{ fontSize: 11 }}>
                    {item.messageCount > 0 ? `${item.messageCount} 条消息` : ''}
                  </Text>
                  <Text type="secondary" style={{ fontSize: 11 }}>
                    {formatRelativeTime(item.updatedAt)}
                  </Text>
                </div>
              </div>

              <Popconfirm
                title="确定删除该对话？"
                onConfirm={(e) => {
                  e?.stopPropagation();
                  handleDelete(item.id);
                }}
                onCancel={(e) => e?.stopPropagation()}
                okText="删除"
                cancelText="取消"
              >
                <Button
                  type="text"
                  size="small"
                  danger
                  icon={<DeleteOutlined />}
                  onClick={(e) => e.stopPropagation()}
                  style={{ opacity: 0.4, marginTop: 2 }}
                  onMouseEnter={(e) => {
                    (e.currentTarget as HTMLElement).style.opacity = '1';
                  }}
                  onMouseLeave={(e) => {
                    (e.currentTarget as HTMLElement).style.opacity = '0.4';
                  }}
                />
              </Popconfirm>
            </div>
          );
        })}
      </div>
    </div>
  );
}
