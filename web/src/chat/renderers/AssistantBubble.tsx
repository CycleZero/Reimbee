import { useState, useEffect, useRef } from 'react';
import { Tag, Button, Input, Typography, Select, App } from 'antd';
import {
  ToolOutlined,
  BulbOutlined,
  BulbFilled,
  CheckOutlined,
  CloseOutlined,
  LoadingOutlined,
  CaretRightOutlined,
  EditOutlined,
} from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { TOOL_LABELS } from '@/utils/constants';
import { useChatStore } from '../stores/chatStore';
import { confirmInvoice } from '@/api';
import type { MessageRendererProps, ToolCallRecord, MessageCard } from '../types';

const { Text } = Typography;

// ============================================
// 共享样式
// ============================================
const markdownStyle: React.CSSProperties = {
  lineHeight: 1.7,
  wordBreak: 'break-word',
};

import type { Components } from 'react-markdown';

const tableBorder = '1px solid #d9d9d9';
const markdownComponents: Components = {
  p: ({ children }) => <p style={markdownStyle}>{children}</p>,
  code: ({ className, children, ...props }: React.ComponentPropsWithoutRef<'code'>) => {
    const isInline = !className;
    return isInline ? (
      <code style={{ background: '#e8e8e8', padding: '2px 6px', borderRadius: 4, fontSize: '0.9em' }} {...props}>
        {children}
      </code>
    ) : (
      <pre style={{ background: '#1e1e1e', color: '#d4d4d4', padding: 12, borderRadius: 8, overflow: 'auto', fontSize: '0.85em' }}>
        <code className={className} {...props}>{children}</code>
      </pre>
    );
  },
  table: ({ children }) => (
    <table style={{ borderCollapse: 'collapse', width: '100%', margin: '8px 0', fontSize: 13 }}>{children}</table>
  ),
  th: ({ children }) => (
    <th style={{ border: tableBorder, padding: '6px 12px', background: '#fafafa', textAlign: 'left', fontWeight: 600 }}>{children}</th>
  ),
  td: ({ children }) => (
    <td style={{ border: tableBorder, padding: '6px 12px' }}>{children}</td>
  ),
};

function formatOutput(output: unknown): string {
  if (output == null) return '';
  try {
    const obj = typeof output === 'string' ? JSON.parse(output) : output;
    return JSON.stringify(obj, null, 2);
  } catch {
    return String(output);
  }
}

// ============================================
// 🧠 思考卡片
// 默认折叠，显示 "思考中..." 或推理进度
// 展开后显示推理 Markdown + 关联工具调用
// ============================================
function ThinkingCard({ card, isStreaming }: { card: MessageCard; isStreaming: boolean }) {
  const [expanded, setExpanded] = useState(false);
  const [animating, setAnimating] = useState(false);
  const hasContent = (card.content?.length ?? 0) > 0;
  const hasTools = (card.toolCalls?.length ?? 0) > 0;
  const thinkingText = card.thinkingText || '思考中...';
  const isDone = thinkingText === '思考完成';
  const prevDone = useRef(isDone);

  useEffect(() => {
    if (isDone && !prevDone.current) {
      setAnimating(true);
      const timer = setTimeout(() => setAnimating(false), 600);
      return () => clearTimeout(timer);
    }
    prevDone.current = isDone;
  }, [isDone]);

  return (
    <div style={{ marginBottom: 6 }}>
      {/* 折叠 / 展开按钮 */}
      <div
        onClick={() => setExpanded((v) => !v)}
        onKeyDown={(e) => { if (e.key === 'Enter') setExpanded((v) => !v); }}
        role="button"
        tabIndex={0}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          padding: '6px 10px',
          background: '#fafafa',
          borderRadius: 8,
          border: '1px solid #f0f0f0',
          cursor: 'pointer',
          userSelect: 'none',
        }}
      >
        <CaretRightOutlined
          style={{
            fontSize: 10,
            color: '#999',
            transition: 'transform 0.2s',
            transform: expanded ? 'rotate(90deg)' : 'rotate(0deg)',
          }}
        />
        <span
          className={animating ? 'thinking-done' : ''}
          style={{
            fontSize: 13,
            color: '#666',
            transition: 'color 0.4s',
            display: 'inline-block',
          }}
        >
          🧠 {thinkingText}
        </span>
        {hasTools && (
          <Tag
            icon={<ToolOutlined />}
            color="default"
            style={{ margin: 0, fontSize: 10, lineHeight: '16px', padding: '0 4px' }}
          >
            {card.toolCalls!.length} 个工具
          </Tag>
        )}
        {isStreaming && !hasContent && (
          <LoadingOutlined style={{ fontSize: 12, color: '#1677ff' }} />
        )}
      </div>

      {/* 展开内容 */}
      {expanded && (
        <div
          style={{
            marginTop: 4,
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
          {hasContent && (
            <div style={{ marginBottom: hasTools ? 12 : 0 }}>
              <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
                {card.content!}
              </ReactMarkdown>
            </div>
          )}
          {hasTools && (
            <div>
              <div style={{ fontWeight: 600, marginBottom: 8, color: '#555', fontSize: 12 }}>
                🛠 推理阶段工具调用
              </div>
              {card.toolCalls!.map((tc, idx) => (
                <div
                  key={tc.id}
                  style={{
                    marginBottom: idx < (card.toolCalls!.length - 1) ? 8 : 0,
                    padding: '8px 10px',
                    background: '#fff',
                    borderRadius: 6,
                    border: '1px solid #f0f0f0',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 4 }}>
                    <Tag
                      color={tc.status === 'running' ? 'processing' : 'success'}
                      style={{ fontSize: 10, lineHeight: '16px', margin: 0 }}
                    >
                      {tc.status === 'running' ? '执行中' : '完成'}
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
                      {formatOutput(tc.output)}
                    </pre>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// 金额分→元的格式化
function formatAmount(fen: unknown): string {
  const n = Number(fen);
  if (isNaN(n) || n === 0) return '';
  return (n / 100).toFixed(2);
}

// ============================================
// 🔧 工具卡片
// 默认折叠（连续工具卡片不占用空间），仅显示一行摘要
// 展开后显示输入参数和输出结果
// ============================================
function ToolCard({ card, isStreaming }: { card: MessageCard; isStreaming: boolean }) {
  const [expanded, setExpanded] = useState(false);
  const label = TOOL_LABELS[card.toolName!] ?? card.toolName!;
  const isRunning = card.status === 'running';
  const isOCR = card.toolName === 'recognize_invoice' && card.status === 'success' && card.output;
  const { message } = App.useApp();
  const sessionId = useChatStore((s) => s.currentSessionId);

  // OCR 编辑状态
  const [editAmount, setEditAmount] = useState('');
  const [editCategory, setEditCategory] = useState('');
  const [editDate, setEditDate] = useState('');
  const [editCode, setEditCode] = useState('');
  const [editNumber, setEditNumber] = useState('');
  const [editSeller, setEditSeller] = useState('');
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);

  // 解析 OCR 输出
  const ocrData = isOCR ? (typeof card.output === 'string' ? JSON.parse(card.output) : card.output) as Record<string, unknown> : null;
  const confidence = (ocrData?.confidence as number) ?? 0;
  const isLowConf = confidence < 0.7;

  // 初始化编辑字段（使用 useEffect 避免 render 期 setState，防止 React #301 无限重渲染）
  const [ocrInitialized, setOcrInitialized] = useState(false);
  useEffect(() => {
    if (isOCR && !ocrInitialized && ocrData) {
      setEditAmount(formatAmount(ocrData.amount));
      setEditCategory((ocrData.category as string) ?? '');
      setEditDate((ocrData.date as string) ?? '');
      setEditCode((ocrData.invoice_code as string) ?? '');
      setEditNumber((ocrData.invoice_number as string) ?? '');
      setEditSeller((ocrData.seller_name as string) ?? '');
      setOcrInitialized(true);
    }
  }, [isOCR, ocrInitialized, ocrData]);

  const handleOCRConfirm = async () => {
    if (!sessionId || !ocrData) return;
    setSaving(true);
    try {
      await confirmInvoice({
        session_id: sessionId,
        image_path: (ocrData.image_path as string) ?? (card.input as Record<string,unknown>)?.image_path as string ?? '',
        amount: editAmount ? Math.round(parseFloat(editAmount) * 100) : undefined,
        category: editCategory || undefined,
        date: editDate || undefined,
        invoice_code: editCode || undefined,
        invoice_number: editNumber || undefined,
        seller_name: editSeller || undefined,
      });
      setSaved(true);
      message.success('票据信息已更新');
    } catch {
      message.error('更新失败');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div style={{ marginBottom: 4 }}>
      <div
        onClick={() => setExpanded((v) => !v)}
        onKeyDown={(e) => { if (e.key === 'Enter') setExpanded((v) => !v); }}
        role="button"
        tabIndex={0}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          padding: '5px 10px',
          background: isRunning ? '#fffbe6' : '#f9fff9',
          borderRadius: 8,
          border: `1px solid ${isRunning ? '#ffe58f' : '#e8f5e9'}`,
          cursor: 'pointer',
          userSelect: 'none',
        }}
      >
        <CaretRightOutlined
          style={{
            fontSize: 10,
            color: '#999',
            transition: 'transform 0.2s',
            transform: expanded ? 'rotate(90deg)' : 'rotate(0deg)',
          }}
        />
        <Tag
          color={isRunning ? 'processing' : card.status === 'error' ? 'error' : 'success'}
          style={{ fontSize: 10, lineHeight: '16px', margin: 0 }}
        >
          {isRunning ? '执行中' : card.status === 'error' ? '失败' : '完成'}
        </Tag>
        <span style={{ fontSize: 13, fontWeight: 500 }}>
          🔧 {label}
        </span>
        {isRunning && (
          <LoadingOutlined style={{ fontSize: 12, color: '#faad14', marginLeft: 'auto' }} />
        )}
      </div>

      {/* 展开内容 */}
      {expanded && (
        <div
          style={{
            marginTop: 4,
            padding: '10px 14px',
            background: '#fafafa',
            borderRadius: 8,
            border: '1px solid #f0f0f0',
            fontSize: 13,
          }}
        >
          {card.input != null && (
            <div style={{ marginBottom: card.output != null ? 8 : 0 }}>
              <div style={{ fontWeight: 600, marginBottom: 4, color: '#555', fontSize: 12 }}>
                📥 输入参数
              </div>
              <pre
                style={{
                  margin: 0,
                  padding: '6px 8px',
                  background: '#fff',
                  borderRadius: 4,
                  border: '1px solid #f0f0f0',
                  fontSize: 11,
                  lineHeight: 1.4,
                  overflow: 'auto',
                  maxHeight: 150,
                  whiteSpace: 'pre-wrap',
                  wordBreak: 'break-all',
                }}
              >
                {typeof card.input === 'string' ? card.input : JSON.stringify(card.input, null, 2)}
              </pre>
            </div>
          )}
          {!isRunning && card.output != null && !isOCR && (
            <div>
              <div style={{ fontWeight: 600, marginBottom: 4, color: '#555', fontSize: 12 }}>
                📤 输出结果
              </div>
              <pre style={{ margin: 0, padding: '6px 8px', background: '#fff', borderRadius: 4, border: '1px solid #f0f0f0', fontSize: 11, lineHeight: 1.4, overflow: 'auto', maxHeight: 200, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                {formatOutput(card.output)}
              </pre>
            </div>
          )}

          {/* OCR 编辑表单 */}
          {isOCR && ocrData ? (
            <div>
              <div style={{ fontWeight: 600, marginBottom: 8, color: '#555', fontSize: 12 }}>
                ✏️ 识别结果（可修改）
                {isLowConf && <Tag color="warning" style={{ marginLeft: 8, fontSize: 10 }}>低置信度 {Math.round(confidence*100)}%</Tag>}
              </div>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
                <div style={{ width: 'calc(50% - 4px)' }}>
                  <div style={{ fontSize: 11, color: '#888', marginBottom: 2 }}>费用类别</div>
                  <Select size="small" value={editCategory || undefined} onChange={setEditCategory} style={{ width: '100%' }}
                    options={['差旅-交通','差旅-住宿','差旅-补助','招待费','办公用品','印刷费','其他'].map(v=>({label:v,value:v}))} />
                </div>
                <div style={{ width: 'calc(50% - 4px)' }}>
                  <div style={{ fontSize: 11, color: '#888', marginBottom: 2 }}>票面金额（元）</div>
                  <Input size="small" value={editAmount} onChange={e => setEditAmount(e.target.value)} placeholder="0.00" />
                </div>
                <div style={{ width: 'calc(50% - 4px)' }}>
                  <div style={{ fontSize: 11, color: '#888', marginBottom: 2 }}>开票日期</div>
                  <Input size="small" value={editDate} onChange={e => setEditDate(e.target.value)} placeholder="YYYY-MM-DD" />
                </div>
                <div style={{ width: 'calc(50% - 4px)' }}>
                  <div style={{ fontSize: 11, color: '#888', marginBottom: 2 }}>销售方</div>
                  <Input size="small" value={editSeller} onChange={e => setEditSeller(e.target.value)} placeholder="销售方名称" />
                </div>
                <div style={{ width: 'calc(50% - 4px)' }}>
                  <div style={{ fontSize: 11, color: '#888', marginBottom: 2 }}>发票代码</div>
                  <Input size="small" value={editCode} onChange={e => setEditCode(e.target.value)} placeholder="发票代码" />
                </div>
                <div style={{ width: 'calc(50% - 4px)' }}>
                  <div style={{ fontSize: 11, color: '#888', marginBottom: 2 }}>发票号码</div>
                  <Input size="small" value={editNumber} onChange={e => setEditNumber(e.target.value)} placeholder="发票号码" />
                </div>
              </div>
              <div style={{ marginTop: 10, display: 'flex', gap: 8 }}>
                <Button type="primary" size="small" icon={<CheckOutlined />} loading={saving} disabled={saved} onClick={handleOCRConfirm}>
                  {saved ? '已确认' : '确认修正'}
                </Button>
                {saved && <span style={{ fontSize: 12, color: '#52c41a', lineHeight: '24px' }}>✅ 已更新到会话</span>}
              </div>
            </div>
          ) : null}
        </div>
      )}
    </div>
  );
}

// ============================================
// 💬 消息卡片 — Markdown 正文，始终可见
// ============================================
function MessageCardView({ card, isStreaming }: { card: MessageCard; isStreaming: boolean }) {
  return (
    <div
      style={{
        background: '#F5F5F5',
        borderRadius: 12,
        padding: '10px 14px',
        marginBottom: 4,
      }}
    >
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
        {card.content || (isStreaming ? '' : '...')}
      </ReactMarkdown>
      {isStreaming && <span className="cursor-blink" />}
    </div>
  );
}

// ============================================
// ⚠️ 中断确认栏
// ============================================
function InterruptBar({ card, messageId }: { card: MessageCard; messageId: string }) {
  const [reason, setReason] = useState('');
  const [resolved, setResolved] = useState(false);
  const triggerApprove = useChatStore((s) => s.triggerApprove);
  const sessionId = useChatStore((s) => s.currentSessionId);
  const interrupt = card.interrupt!;
  const isPending = interrupt.status === 'pending' && !resolved;

  const handle = (approved: boolean) => {
    if (!sessionId) return;
    setResolved(true);
    triggerApprove({ session_id: sessionId, approved, reason });
  };

  if (!isPending) {
    const isApproved = interrupt.status === 'approved' || (resolved && interrupt.status !== 'rejected');
    return (
      <div
        style={{
          marginBottom: 4,
          padding: '6px 12px',
          background: '#f5f5f5',
          borderRadius: 8,
          border: '1px solid #e8e8e8',
          fontSize: 12,
          color: '#999',
          display: 'flex',
          alignItems: 'center',
          gap: 8,
        }}
      >
        <span>{isApproved ? '✅ 已确认' : '❌ 已取消'}</span>
        <span>— {TOOL_LABELS[interrupt.toolName] ?? interrupt.toolName}</span>
      </div>
    );
  }

  const toolLabel = TOOL_LABELS[interrupt.toolName] ?? interrupt.toolName;

  return (
    <div
      style={{
        marginBottom: 4,
        padding: '12px 14px',
        background: '#fffbe6',
        borderRadius: 10,
        border: '1px solid #ffe58f',
        fontSize: 13,
      }}
    >
      <div style={{ fontWeight: 600, marginBottom: 6, color: '#ad6800', fontSize: 14 }}>
        ⚠️ 需要确认 — {toolLabel}
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
        <Button size="small" icon={<CloseOutlined />} onClick={() => handle(false)}>
          取消
        </Button>
        <Button size="small" type="primary" icon={<CheckOutlined />} onClick={() => handle(true)}>
          确认
        </Button>
      </div>
    </div>
  );
}

// ============================================
// AssistantBubble — 主渲染组件
// 仅渲染 cards[]，cards 为空时显示纯文本回退
// ============================================
export function AssistantBubble({ message }: MessageRendererProps) {
  const cards = message.cards;
  const isStreaming = message.isStreaming ?? false;

  // 卡片模式 — 按顺序渲染
  if (cards && cards.length > 0) {
    return (
      <div style={{ display: 'flex', gap: 12, flexDirection: 'row', padding: '8px 16px' }}>
        <div style={{ maxWidth: '75%' }}>
          {cards.map((card, idx) => {
            switch (card.type) {
              case 'thinking':
                return (
                  <ThinkingCard
                    key={`thinking-${idx}`}
                    card={card}
                    isStreaming={isStreaming && idx === cards.length - 1}
                  />
                );
              case 'tool':
                return (
                  <ToolCard
                    key={`tool-${idx}`}
                    card={card}
                    isStreaming={isStreaming && idx === cards.length - 1 && card.status === 'running'}
                  />
                );
              case 'message':
                return (
                  <MessageCardView
                    key={`msg-${idx}`}
                    card={card}
                    isStreaming={isStreaming && idx === cards.length - 1}
                  />
                );
              case 'interrupt':
                return (
                  <InterruptBar
                    key={`interrupt-${idx}`}
                    card={card}
                    messageId={message.id}
                  />
                );
            }
          })}
        </div>
      </div>
    );
  }

  // 回退：无卡片时显示纯文本（历史消息兼容）
  if (message.content) {
    return (
      <div style={{ display: 'flex', gap: 12, flexDirection: 'row', padding: '8px 16px' }}>
        <div style={{ maxWidth: '75%' }}>
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
              }}
            >
              {message.content}
            </ReactMarkdown>
          </div>
        </div>
      </div>
    );
  }

  return null;
}
