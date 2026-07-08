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

      const fullMsg = uploadedFile
        ? `${msg}\n[已上传票据: ${uploadedFile.path}]`
        : msg;

      store.addUserMessage(fullMsg);
      setPendingMessage(fullMsg);
      setUploadedFile(null);
    },
    [sessionId, connectionStatus, uploadedFile, store, navigate, antMsg],
  );

  const isDisabled = connectionStatus === 'connecting' || connectionStatus === 'connected';

  return (
    <div style={{ height: 'calc(100vh - 64px - 48px - 48px)', display: 'flex', overflow: 'hidden' }}>
      <SessionList />

      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0, minHeight: 0 }}>
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
