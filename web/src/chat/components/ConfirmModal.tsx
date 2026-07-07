import { Modal, Button, Space } from 'antd';
import { useChatStore } from '../stores/chatStore';

/**
 * 操作确认弹窗
 * 当后端 SSE 发送 confirm_required 事件时显示
 */
export function ConfirmModal() {
  const { confirmPrompt, setConfirmPrompt, addUserMessage } = useChatStore();
  if (!confirmPrompt) return null;

  const handle = (action: string) => {
    addUserMessage(`${action}：${confirmPrompt.action}`);
    setConfirmPrompt(null);
  };

  return (
    <Modal
      title="确认操作"
      open
      onCancel={() => handle('取消')}
      footer={
        <Space>
          <Button onClick={() => handle('取消')}>取消</Button>
          <Button type="primary" onClick={() => handle('确认')}>
            确认
          </Button>
        </Space>
      }
    >
      <p>{confirmPrompt.prompt}</p>
    </Modal>
  );
}
