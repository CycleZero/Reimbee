import { Input, Button, Space } from 'antd';
import { SendOutlined } from '@ant-design/icons';
import { useState, useRef } from 'react';

interface Props {
  onSend: (message: string) => void;
  disabled?: boolean;
}

export function ChatInput({ onSend, disabled }: Props) {
  const [value, setValue] = useState('');
  const inputRef = useRef<any>(null);

  const handleSend = () => {
    const msg = value.trim();
    if (!msg || disabled) return;
    onSend(msg);
    setValue('');
    inputRef.current?.focus();
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  return (
    <div style={{ padding: '12px 16px', borderTop: '1px solid #F0F0F0' }}>
      <Space.Compact style={{ width: '100%' }}>
        <Input.TextArea
          ref={inputRef}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="输入消息...（Shift+Enter 换行）"
          autoSize={{ minRows: 1, maxRows: 4 }}
          disabled={disabled}
          style={{ borderRadius: '12px 0 0 12px' }}
        />
        <Button
          type="primary"
          icon={<SendOutlined />}
          onClick={handleSend}
          disabled={disabled || !value.trim()}
          style={{ borderRadius: '0 12px 12px 0', height: 'auto' }}
        />
      </Space.Compact>
    </div>
  );
}
