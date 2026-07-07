import { Card, Tag } from 'antd';
import {
  ToolOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  LoadingOutlined,
} from '@ant-design/icons';
import type { ToolRendererProps } from '../types';
import { TOOL_LABELS } from '@/utils/constants';

/**
 * 工具调用卡片渲染器
 * 默认渲染所有工具调用，可通过 registry 按 toolName 注册专用渲染器
 */
export function ToolCallCard({ call }: ToolRendererProps) {
  const label = TOOL_LABELS[call.toolName] ?? call.toolName;

  const statusIcon = {
    running: <LoadingOutlined spin style={{ color: '#1677FF' }} />,
    success: <CheckCircleOutlined style={{ color: '#52C41A' }} />,
    error: <CloseCircleOutlined style={{ color: '#FF4D4F' }} />,
  }[call.status];

  const statusLabel = {
    running: '执行中',
    success: '完成',
    error: '失败',
  }[call.status];

  return (
    <Card size="small" styles={{ body: { padding: '8px 12px' } }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <ToolOutlined />
        <span style={{ fontWeight: 500 }}>{label}</span>
        {statusIcon}
        <Tag>{statusLabel}</Tag>
      </div>
      {call.status === 'error' && call.errorMessage && (
        <p style={{ color: '#FF4D4F', fontSize: 12, marginTop: 4 }}>
          {call.errorMessage}
        </p>
      )}
    </Card>
  );
}
