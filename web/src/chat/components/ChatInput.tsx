import { Input, Button, Space } from 'antd';
import { SendOutlined } from '@ant-design/icons';
import { useState, useRef, type ReactNode } from 'react';

interface Props {
  onSend: (message: string) => void;
  disabled?: boolean;
  placeholder?: string;
  /** 输入框左侧插槽（如附件按钮） */
  prefix?: ReactNode;
  /** 发送按钮右侧插槽（如语音按钮） */
  suffix?: ReactNode;
}

/**
 * 聊天输入框
 * - 支持 Shift+Enter 换行，Enter 发送
 * - prefix/suffix 插槽用于扩展工具栏
 * - 自动聚焦
 */
export function ChatInput({
  onSend,
  disabled = false,
  placeholder = '输入消息...（Shift+Enter 换行）',
  prefix,
  suffix,
}: Props) {
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
      {prefix && <div style={{ marginBottom: 8 }}>{prefix}</div>}
      <Space.Compact style={{ width: '100%' }}>
        <Input.TextArea
          ref={inputRef}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={placeholder}
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
      {suffix && <div style={{ marginTop: 8 }}>{suffix}</div>}
    </div>
  );
}
