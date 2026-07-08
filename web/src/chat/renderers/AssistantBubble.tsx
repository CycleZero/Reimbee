import { useState } from 'react';
import { Avatar, Tag, Button, Input } from 'antd';
import { RobotOutlined, ToolOutlined, BulbOutlined, BulbFilled, CheckOutlined, CloseOutlined } from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { TOOL_LABELS } from '@/utils/constants';
import { useChatStore } from '../stores/chatStore';
import type { MessageRendererProps, ToolCallRecord, MessageInterrupt } from '../types';

const markdownStyle: React.CSSProperties = {
  lineHeight: 1.7,
  wordBreak: 'break-word',
};

function formatToolOutput(call: ToolCallRecord): string {
  if (!call.output) return '';
  try {
    const obj = typeof call.output === 'string' ? JSON.parse(call.output) : call.output;
    return JSON.stringify(obj, null, 2);
  } catch {
    return String(call.output);
  }
}

/** Inline 中断确认栏 */
function InterruptBar({ interrupt, messageId }: { interrupt: MessageInterrupt; messageId: string }) {
  const [reason, setReason] = useState('');
  const [expanded, setExpanded] = useState(true);
  const triggerApprove = useChatStore((s) => s.triggerApprove);
  const sessionId = useChatStore((s) => s.currentSessionId);
  const isPending = interrupt.status === 'pending';

  const handle = (approved: boolean) => {
    if (!sessionId) return;
    useChatStore.setState((prev) => ({
      messages: prev.messages.map((m) =>
        m.id === messageId
          ? { ...m, interrupt: { ...interrupt, status: approved ? 'approved' : 'rejected' } as MessageInterrupt }
          : m,
      ),
    }));
    triggerApprove({ session_id: sessionId, approved, reason });
  };

  if (!isPending) {
    // 已确认/已拒绝 — 灰色收起状态
    return (
      <div
        style={{
          marginTop: 6,
          padding: '6px 12px',
          background: '#f5f5f5',
          borderRadius: 8,
          border: '1px solid #e8e8e8',
          fontSize: 12,
          color: '#999',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          cursor: 'pointer',
        }}
        onClick={() => setExpanded((v) => !v)}
        onKeyDown={(e) => { if (e.key === 'Enter') setExpanded((v) => !v); }}
        role="button"
        tabIndex={0}
      >
        <span>
          {interrupt.status === 'approved' ? '✅ 已确认' : '❌ 已取消'} — {TOOL_LABELS[interrupt.toolName] ?? interrupt.toolName}
        </span>
        <span style={{ fontSize: 11 }}>{expanded ? '收起' : '展开'}</span>
      </div>
    );
  }

  // pending — 黄色醒目展开
  const toolLabel = TOOL_LABELS[interrupt.toolName] ?? interrupt.toolName;

  return (
    <div
      style={{
        marginTop: 6,
        padding: '10px 14px',
        background: '#fffbe6',
        borderRadius: 8,
        border: '1px solid #ffe58f',
        fontSize: 13,
      }}
    >
      <div style={{ fontWeight: 600, marginBottom: 6, color: '#ad6800' }}>
        ⚠ {toolLabel}
      </div>
      <div style={{ marginBottom: 10, color: '#8c6900', whiteSpace: 'pre-wrap', lineHeight: 1.6 }}>
        {interrupt.reason}
      </div>

      <div style={{ marginBottom: 8 }}>
        <Input.TextArea
          rows={2}
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          placeholder="补充说明（可选）"
          style={{ fontSize: 12 }}
        />
      </div>

      <div style={{ display: 'flex', gap: 8 }}>
        <Button
          size="small"
          icon={<CloseOutlined />}
          onClick={() => handle(false)}
        >
          取消
        </Button>
        <Button
          size="small"
          type="primary"
          icon={<CheckOutlined />}
          onClick={() => handle(true)}
        >
          确认
        </Button>
      </div>
    </div>
  );
}

export function AssistantBubble({ message }: MessageRendererProps) {
  const toolCalls = message.toolCalls ?? [];
  const toolCount = toolCalls.length;
  const hasRunning = toolCalls.some((tc) => tc.status === 'running');
  const hasReasoning = (message.reasoning?.length ?? 0) > 0 || toolCount > 0;
  const [reasoningOpen, setReasoningOpen] = useState(false);

  return (
    <div style={{ display: 'flex', gap: 12, flexDirection: 'row', padding: '8px 16px' }}>
      <Avatar
        icon={<RobotOutlined />}
        style={{ backgroundColor: '#52C41A', flexShrink: 0 }}
      />
      <div style={{ maxWidth: '75%' }}>
        {/* Section 1: 思考过程 — 可折叠 */}
        {hasReasoning && (
          <div style={{ marginBottom: 4 }}>
            <Button
              type="text"
              size="small"
              icon={reasoningOpen ? <BulbFilled style={{ color: '#faad14' }} /> : <BulbOutlined />}
              onClick={() => setReasoningOpen((v) => !v)}
              style={{ padding: '0 4px', fontSize: 12, color: '#888' }}
            >
              {reasoningOpen ? '收起思考过程' : '查看思考过程'}
              {toolCount > 0 && (
                <Tag
                  icon={<ToolOutlined />}
                  color="default"
                  style={{ marginLeft: 6, fontSize: 10, lineHeight: '16px', padding: '0 4px' }}
                >
                  {toolCount}
                </Tag>
              )}
              {message.isStreaming && hasRunning && (
                <span
                  className="cursor-blink"
                  style={{ display: 'inline-block', width: 6, height: 6, borderRadius: '50%', background: '#1677ff', marginLeft: 4 }}
                />
              )}
            </Button>

            {reasoningOpen && (
              <div
                style={{
                  marginTop: 6,
                  padding: '10px 14px',
                  background: '#fafafa',
                  borderRadius: 8,
                  border: '1px solid #f0f0f0',
                  fontSize: 13,
                  lineHeight: 1.7,
                  color: '#666',
                  maxHeight: 400,
                  overflow: 'auto',
                }}
              >
                {(message.reasoning?.length ?? 0) > 0 && (
                  <div style={{ marginBottom: toolCount > 0 ? 12 : 0 }}>
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>
                      {message.reasoning!}
                    </ReactMarkdown>
                  </div>
                )}

                {toolCount > 0 && (
                  <div>
                    <div style={{ fontWeight: 600, marginBottom: 8, color: '#555', fontSize: 12 }}>
                      🛠 工具调用明细
                    </div>
                    {toolCalls.map((tc, idx) => (
                      <div
                        key={tc.id}
                        style={{
                          marginBottom: idx < toolCalls.length - 1 ? 8 : 0,
                          padding: '8px 10px',
                          background: '#fff',
                          borderRadius: 6,
                          border: '1px solid #f0f0f0',
                        }}
                      >
                        <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 4 }}>
                          <Tag
                            color={tc.status === 'running' ? 'processing' : tc.status === 'error' ? 'error' : 'success'}
                            style={{ fontSize: 10, lineHeight: '16px', margin: 0 }}
                          >
                            {tc.status === 'running' ? '执行中' : tc.status === 'error' ? '失败' : '完成'}
                          </Tag>
                          <span style={{ fontWeight: 500, fontSize: 12 }}>
                            {TOOL_LABELS[tc.toolName] ?? tc.toolName}
                          </span>
                        </div>
                        {tc.status !== 'running' && tc.output != null && (
                          <pre
                            style={{
                              margin: 0,
                              padding: '6px 8px',
                              background: '#f5f5f5',
                              borderRadius: 4,
                              fontSize: 11,
                              lineHeight: 1.4,
                              overflow: 'auto',
                              maxHeight: 200,
                              whiteSpace: 'pre-wrap',
                              wordBreak: 'break-all',
                            }}
                          >
                            {formatToolOutput(tc)}
                          </pre>
                        )}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}
          </div>
        )}

        {/* Section 2: 工具调用数量标签 */}
        {toolCount > 0 && (
          <div style={{ marginBottom: 4, display: 'flex', alignItems: 'center', gap: 4 }}>
            <Tag icon={<ToolOutlined />} color="default" style={{ fontSize: 11, lineHeight: '18px' }}>
              {toolCount} 个工具调用
            </Tag>
            {message.isStreaming && hasRunning && (
              <span
                className="cursor-blink"
                style={{ display: 'inline-block', width: 6, height: 6, borderRadius: '50%', background: '#1677ff' }}
              />
            )}
          </div>
        )}

        {/* Section 3: 回复内容 */}
        <div
          style={{
            background: '#F5F5F5',
            borderRadius: 12,
            padding: '10px 14px',
          }}
        >
          <ReactMarkdown
            remarkPlugins={[remarkGfm]}
            components={{
              p: ({ children }) => <p style={markdownStyle}>{children}</p>,
              code: ({ className, children, ...props }) => {
                const isInline = !className;
                const content = children as React.ReactNode;
                return isInline ? (
                  <code style={{ background: '#e8e8e8', padding: '2px 6px', borderRadius: 4, fontSize: '0.9em' }} {...props}>
                    {content}
                  </code>
                ) : (
                  <pre style={{ background: '#1e1e1e', color: '#d4d4d4', padding: 12, borderRadius: 8, overflow: 'auto', fontSize: '0.85em' }}>
                    <code className={className} {...props}>{content}</code>
                  </pre>
                );
              },
            }}
          >
            {message.content || (message.isStreaming ? '' : '...')}
          </ReactMarkdown>
          {message.isStreaming && <span className="cursor-blink" />}
        </div>

        {/* Inline 中断确认栏 — 放在 content 下方 */}
        {message.interrupt && (
          <InterruptBar interrupt={message.interrupt} messageId={message.id} />
        )}
      </div>
    </div>
  );
}
