import type { ReactNode } from 'react';
import { MessageList } from './components/MessageList';
import { ChatInput } from './components/ChatInput';
import './renderers/index';

interface Props {
  header?: ReactNode;
  inputPrefix?: ReactNode;
  inputSuffix?: ReactNode;
  placeholder?: string;
  onSend: (message: string) => void;
  disabled?: boolean;
}

export function ChatLayout({
  header,
  inputPrefix,
  inputSuffix,
  placeholder,
  onSend,
  disabled,
}: Props) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {header && <div style={{ padding: '12px 16px 0' }}>{header}</div>}
      <MessageList />
      <ChatInput
        onSend={onSend}
        disabled={disabled}
        placeholder={placeholder}
        prefix={inputPrefix}
        suffix={inputSuffix}
      />
    </div>
  );
}
