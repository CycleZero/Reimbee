import { useState, useCallback, useEffect } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { App } from 'antd';
import { ChatLayout } from '@/chat/ChatLayout';
import { PhaseIndicator } from '@/chat/components/PhaseIndicator';
import { UploadButton } from '@/chat/components/UploadButton';
import { SessionList } from '@/chat/components/SessionList';
import { useChatStore } from '@/chat/stores/chatStore';
import { useChatStream } from '@/chat/useChatStream';

interface UploadedFile {
  path: string;
  url: string;
  name: string;
}

export default function Chat() {
  const navigate = useNavigate();
  const { sessionId: urlSessionId } = useParams<{ sessionId: string }>();
  const { message: antMsg } = App.useApp();

  const [pendingMessage, setPendingMessage] = useState<string | null>(null);
  const [uploadedFile, setUploadedFile] = useState<UploadedFile | null>(null);
  const store = useChatStore();

  const sessionId = store.currentSessionId;
  const connectionStatus = store.connectionStatus;

  // 页面挂载时同步 URL 中的 sessionId
  useEffect(() => {
    if (urlSessionId) {
      if (urlSessionId !== store.currentSessionId) {
        store.switchSession(urlSessionId);
      }
    }
    store.loadSessions();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // SSE 连接
  useChatStream(sessionId, pendingMessage);

  // 发送消息
  const handleSend = useCallback(
    (msg: string) => {
      if (connectionStatus === 'connecting' || connectionStatus === 'connected') {
        antMsg.warning('正在回复中，请稍候...');
        return;
      }

      let sid = sessionId;
      if (!sid) {
        sid = store.initSession();
        navigate(`/chat/${sid}`, { replace: true });
      }

      // 拼接已上传的票据路径，供 Agent 调用 OCR 工具
      const fullMsg = uploadedFile
        ? `${msg}\n[已上传票据: ${uploadedFile.path}]`
        : msg;

      store.addUserMessage(fullMsg);
      setPendingMessage(fullMsg);
      setUploadedFile(null); // 发送后清空已上传票据
    },
    [sessionId, connectionStatus, uploadedFile, store, navigate, antMsg],
  );

  const isDisabled = connectionStatus === 'connecting' || connectionStatus === 'connected';

  return (
    <div style={{ height: 'calc(100vh - 120px)', display: 'flex' }}>
      {/* 左侧会话列表 */}
      <SessionList />

      {/* 右侧聊天区域 */}
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0 }}>
        <ChatLayout
          header={<PhaseIndicator />}
          inputPrefix={
            <UploadButton
              value={uploadedFile}
              onChange={setUploadedFile}
              disabled={isDisabled}
            />
          }
          onSend={handleSend}
          disabled={isDisabled}
        />
      </div>
    </div>
  );
}
