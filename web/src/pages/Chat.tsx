import { useState, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { App } from 'antd';
import { ChatLayout } from '@/chat/ChatLayout';
import { PhaseIndicator } from '@/chat/components/PhaseIndicator';
import { UploadButton } from '@/chat/components/UploadButton';
import { useChatStore } from '@/chat/stores/chatStore';
import { useChatStream } from '@/chat/useChatStream';

interface UploadedFile {
  path: string;
  url: string;
  name: string;
}

export default function Chat() {
  const navigate = useNavigate();
  const { message: antMsg } = App.useApp();

  const [pendingMessage, setPendingMessage] = useState<string | null>(null);
  const [uploadedFile, setUploadedFile] = useState<UploadedFile | null>(null);
  const store = useChatStore();

  const sessionId = store.currentSessionId;
  const connectionStatus = store.connectionStatus;

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
    <div style={{ height: 'calc(100vh - 120px)', display: 'flex', flexDirection: 'column' }}>
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
  );
}
