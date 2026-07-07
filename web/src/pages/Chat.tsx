import { useState, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { App } from 'antd';
import { ChatLayout } from '@/chat/ChatLayout';
import { PhaseIndicator } from '@/chat/components/PhaseIndicator';
import { useChatStore } from '@/chat/stores/chatStore';
import { useChatStream } from '@/chat/useChatStream';

export default function Chat() {
  const { sessionId: paramSessionId } = useParams<{ sessionId?: string }>();
  const navigate = useNavigate();
  const { message: antMsg } = App.useApp();

  const [pendingMessage, setPendingMessage] = useState<string | null>(null);
  const store = useChatStore();

  // 延迟初始化：首条消息发送时才创建 session
  const sessionId = store.currentSessionId;
  const connectionStatus = store.connectionStatus;

  // SSE 连接（仅在 sessionId + pendingMessage 都存在时建立）
  useChatStream(sessionId, pendingMessage);

  // 发送消息
  const handleSend = useCallback(
    (msg: string) => {
      let sid = sessionId;
      if (!sid) {
        sid = store.initSession(paramSessionId);
        navigate(`/chat/${sid}`, { replace: true });
      }

      store.addUserMessage(msg);

      if (connectionStatus === 'connecting' || connectionStatus === 'connected') {
        antMsg.warning('正在回复中，请稍候...');
        return;
      }

      setPendingMessage(msg);
    },
    [sessionId, paramSessionId, connectionStatus, store, navigate, antMsg],
  );

  const isDisabled = connectionStatus === 'connecting' || connectionStatus === 'connected';

  return (
    <div style={{ height: 'calc(100vh - 120px)', display: 'flex', flexDirection: 'column' }}>
      <ChatLayout
        header={<PhaseIndicator />}
        onSend={handleSend}
        disabled={isDisabled}
      />
    </div>
  );
}
