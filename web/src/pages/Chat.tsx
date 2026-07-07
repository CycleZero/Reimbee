import { useState, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { App } from 'antd';
import { ChatPanel } from '@/components/chat/ChatPanel';
import { useChatStore } from '@/stores/chatStore';
import { useSSE } from '@/hooks/useSSE';

export default function Chat() {
  const { sessionId: paramSessionId } = useParams<{ sessionId?: string }>();
  const navigate = useNavigate();
  const { message: antMsg } = App.useApp();

  const [sessionId, setSessionId] = useState<string | null>(null);
  const [pendingMessage, setPendingMessage] = useState<string | null>(null);

  const store = useChatStore();
  const { connectionStatus, initSession, addUserMessage } = store;

  // SSE 连接管理
  useSSE(sessionId, pendingMessage);

  // 发送消息
  const handleSend = useCallback(
    (msg: string) => {
      let sid = sessionId;
      if (!sid) {
        sid = initSession(paramSessionId);
        setSessionId(sid);
        navigate(`/chat/${sid}`, { replace: true });
      }

      addUserMessage(msg);

      if (connectionStatus === 'connecting' || connectionStatus === 'connected') {
        antMsg.warning('正在回复中，请稍候...');
        return;
      }

      setPendingMessage(msg);
    },
    [sessionId, paramSessionId, connectionStatus, initSession, addUserMessage, navigate, antMsg],
  );

  const isDisabled = connectionStatus === 'connecting' || connectionStatus === 'connected';

  return (
    <div style={{ height: 'calc(100vh - 120px)', display: 'flex', flexDirection: 'column' }}>
      <ChatPanel onSend={handleSend} disabled={isDisabled} />
    </div>
  );
}
