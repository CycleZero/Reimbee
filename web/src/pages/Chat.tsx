import { useState, useCallback, useEffect } from 'react';
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

  // ── 组件挂载时初始化会话 ──
  // 如果 URL 中有 sessionId（如 /chat/abc123），则复用已有会话
  // 否则等待用户发送首条消息时通过 handleSend 创建新会话
  useEffect(() => {
    if (paramSessionId && !store.currentSessionId) {
      store.initSession(paramSessionId);
    }
  }, [paramSessionId]);

  const sessionId = store.currentSessionId;
  const connectionStatus = store.connectionStatus;

  // SSE 连接（仅在 sessionId + pendingMessage 都存在时建立）
  useChatStream(sessionId, pendingMessage);

  // 发送消息
  const handleSend = useCallback(
    (msg: string) => {
      // 先检查连接状态，防止重复发送
      if (connectionStatus === 'connecting' || connectionStatus === 'connected') {
        antMsg.warning('正在回复中，请稍候...');
        return;
      }

      // 确保会话已创建（首条消息时生成新 sessionId）
      let sid = sessionId;
      if (!sid) {
        sid = store.initSession(paramSessionId);
        navigate(`/chat/${sid}`, { replace: true });
      }

      // 添加用户消息到 store → 触发 SSE 连接
      store.addUserMessage(msg);
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
