import { useState, useCallback, useEffect } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { App } from 'antd';
import { ChatLayout } from '@/chat/ChatLayout';
import { UploadButton, type UploadedFile } from '@/chat/components/UploadButton';
import { SessionList } from '@/chat/components/SessionList';
import { useChatStore } from '@/chat/stores/chatStore';
import { useChatStream } from '@/chat/useChatStream';

export default function Chat() {
  const navigate = useNavigate();
  const { sessionId: urlSessionId } = useParams<{ sessionId: string }>();
  const { message: antMsg } = App.useApp();

  const [pendingMessage, setPendingMessage] = useState<string | null>(null);
  const [uploadedFiles, setUploadedFiles] = useState<UploadedFile[]>([]);
  const store = useChatStore();

  const sessionId = store.currentSessionId;
  const connectionStatus = store.connectionStatus;
  const approveSignal = store.approveSignal;

  useEffect(() => {
    if (urlSessionId) {
      if (urlSessionId !== store.currentSessionId) {
        store.switchSession(urlSessionId);
      }
    }
    store.loadSessions();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const approveTrigger = approveSignal
    ? { payload: approveSignal.payload, version: approveSignal.version }
    : null;

  useChatStream(sessionId, pendingMessage, undefined, approveTrigger);

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

      // 拼接图片路径到消息末尾（每张一行）
      const imageLines = uploadedFiles
        .map((f) => `[已上传票据: ${f.path}]`)
        .join('\n');
      const fullMsg = imageLines ? `${msg}\n${imageLines}` : msg;

      store.addUserMessage(fullMsg);
      setPendingMessage(fullMsg);
      setUploadedFiles([]);
    },
    [sessionId, connectionStatus, uploadedFiles, store, navigate, antMsg],
  );

  const isDisabled = connectionStatus === 'connecting' || connectionStatus === 'connected';

  return (
    <div style={{ height: 'calc(100vh - 64px - 48px - 48px)', display: 'flex', overflow: 'hidden' }}>
      <SessionList />

      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0, minHeight: 0 }}>
        <ChatLayout
          inputPrefix={
            <UploadButton
              value={uploadedFiles}
              onChange={setUploadedFiles}
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
