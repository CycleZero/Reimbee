import { Table, Button, Space, Modal, Form, Input, App, Card, Empty, Skeleton, Spin } from 'antd';
import { useEffect, useRef, useState } from 'react';
import {
  listPendingReimbursements,
  approveReimbursement,
  rejectReimbursement,
  getApprovalProgress,
} from '@/api';
import { AmountText } from '@/components/common/AmountText';
import { StatusTag } from '@/components/common/StatusTag';
import type { Reimbursement, ApprovalRecord } from '@/types/models';

/** 审批动作中文映射 */
const ACTION_LABELS: Record<string, string> = {
  pending: '待审批',
  approved: '已通过',
  rejected: '已驳回',
};

export default function PendingApprovals() {
  const [data, setData] = useState<Reimbursement[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshKey, setRefreshKey] = useState(0);
  const [rejectVisible, setRejectVisible] = useState(false);
  const [rejectTargetId, setRejectTargetId] = useState<number | null>(null);
  /** 展开行审批记录缓存，useRef 避免重复请求 */
  const expandedCache = useRef<Map<number, ApprovalRecord[]>>(new Map());
  /** 正在加载审批记录的行 ID 集合 */
  const [expandedLoadingKeys, setExpandedLoadingKeys] = useState<Set<number>>(new Set());
  const [form] = Form.useForm();
  const { message, modal } = App.useApp();
  const messageRef = useRef(message);
  messageRef.current = message;

  // ---------- 加载待审批列表 ----------

  useEffect(() => {
    (async () => {
      setLoading(true);
      try {
        const result = await listPendingReimbursements();
        setData(result);
      } catch (err) {
        messageRef.current.error(err instanceof Error ? err.message : '加载失败');
      } finally {
        setLoading(false);
      }
    })();
  }, [refreshKey]);

  // ---------- 审批操作 ----------

  const handleApprove = async (id: number) => {
    try {
      await approveReimbursement(id);
      message.success('审批通过');
      setData((prev) => prev.filter((r) => r.id !== id));
      expandedCache.current.delete(id);
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败');
      // 409 等错误时刷新列表
      setRefreshKey((k) => k + 1);
    }
  };

  const handleReject = async () => {
    try {
      const values = await form.validateFields();
      if (rejectTargetId === null) return;
      await rejectReimbursement(rejectTargetId, values.reason);
      message.success('已驳回');
      setData((prev) => prev.filter((r) => r.id !== rejectTargetId));
      expandedCache.current.delete(rejectTargetId);
      setRejectVisible(false);
      setRejectTargetId(null);
      form.resetFields();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败');
      if (err instanceof Error) {
        // API 错误时刷新列表并关闭弹窗
        setRefreshKey((k) => k + 1);
        setRejectVisible(false);
        setRejectTargetId(null);
        form.resetFields();
      }
      // 表单验证失败时保持弹窗打开，Ant Design 已显示字段级错误
    }
  };

  // ---------- 驳回弹窗 ----------

  const handleCancelReject = () => {
    setRejectVisible(false);
    setRejectTargetId(null);
    form.resetFields();
  };

  // ---------- 展开行：加载审批记录 ----------

  const loadApprovalRecords = async (id: number) => {
    if (expandedCache.current.has(id)) return;
    setExpandedLoadingKeys((prev) => {
      const next = new Set(prev);
      next.add(id);
      return next;
    });
    try {
      const records = await getApprovalProgress(id);
      expandedCache.current.set(id, records);
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载审批记录失败');
    } finally {
      setExpandedLoadingKeys((prev) => {
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
    }
  };

  const handleExpand = (expanded: boolean, record: Reimbursement) => {
    if (expanded && !expandedCache.current.has(record.id)) {
      loadApprovalRecords(record.id);
    }
  };

  // ---------- 表格列定义 ----------

  const columns = [
    {
      title: '单号',
      dataIndex: 'reimbursement_no',
      key: 'no',
      render: (v: string) => <span>{v}</span>,
    },
    { title: '申请人', dataIndex: 'employee_name', key: 'name' },
    { title: '部门', dataIndex: 'department', key: 'dept' },
    {
      title: '金额',
      dataIndex: 'total_amount',
      key: 'amount',
      render: (v: number) => <AmountText amount={v} />,
    },
    { title: '事由', dataIndex: 'submit_note', key: 'note', ellipsis: true },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (v: string) => <StatusTag status={v} />,
    },
    {
      title: '操作',
      key: 'action',
      render: (_: unknown, record: Reimbursement) => (
        <Space>
          <Button
            type="primary"
            size="small"
            onClick={() => {
              modal.confirm({
                title: '确认通过',
                content: '确认通过该报销单？',
                onOk: () => handleApprove(record.id),
              });
            }}
          >
            通过
          </Button>
          <Button
            danger
            size="small"
            onClick={() => {
              setRejectTargetId(record.id);
              setRejectVisible(true);
            }}
          >
            驳回
          </Button>
        </Space>
      ),
    },
  ];

  // ---------- 展开行渲染 ----------

  const expandedRowRender = (record: Reimbursement) => {
    const cached = expandedCache.current.get(record.id);
    const isLoading = expandedLoadingKeys.has(record.id);

    const invoiceColumns = [
      { title: '类别', dataIndex: 'category', key: 'cat' },
      {
        title: '金额',
        dataIndex: 'amount',
        key: 'invAmount',
        render: (v: number) => <AmountText amount={v} />,
      },
      { title: '日期', dataIndex: 'invoice_date', key: 'date' },
    ];

    const approvalColumns = [
      { title: '审批人', dataIndex: 'approver_name', key: 'approver' },
      {
        title: '动作',
        dataIndex: 'action',
        key: 'act',
        render: (v: string) => ACTION_LABELS[v] ?? v,
      },
      {
        title: '意见',
        dataIndex: 'comment',
        key: 'comment',
        render: (v: string | undefined) => v || '—',
      },
      {
        title: '时间',
        dataIndex: 'action_at',
        key: 'time',
        render: (v: string | undefined) => v || '—',
      },
    ];

    return (
      <div style={{ padding: '0 24px 16px' }}>
        <h4>报销明细</h4>
        <Table
          columns={[
            { title: '明细', dataIndex: 'item_desc', key: 'item' },
            { title: '类别', dataIndex: 'category', key: 'cat' },
            { title: '票面金额', dataIndex: 'amount', key: 'invAmount', render: (v: number) => <AmountText amount={v} /> },
            { title: '日期', dataIndex: 'invoice_date', key: 'date' },
          ]}
          dataSource={record.items?.flatMap(item =>
            item.receipts?.map(rct => ({ ...rct, item_desc: item.category })) ?? []
          ) ?? []}
          rowKey="id"
          size="small"
          pagination={false}
        />
        <h4 style={{ marginTop: 16 }}>审批记录</h4>
        {isLoading || cached === undefined ? (
          <div style={{ textAlign: 'center', padding: 24 }}>
            <Spin />
          </div>
        ) : (
          <Table
            columns={approvalColumns}
            dataSource={cached}
            rowKey="id"
            size="small"
            pagination={false}
          />
        )}
      </div>
    );
  };

  // ---------- 加载态 / 空态 ----------

  if (loading) {
    return <Skeleton active paragraph={{ rows: 5 }} />;
  }

  if (data.length === 0) {
    return <Empty description="暂无待审批报销单" />;
  }

  // ---------- 主渲染 ----------

  return (
    <>
      <Card title="待审批报销单">
        <Table
          columns={columns}
          dataSource={data}
          rowKey="id"
          pagination={false}
          expandable={{
            expandedRowRender,
            onExpand: handleExpand,
          }}
        />
      </Card>

      <Modal
        title="驳回报销单"
        open={rejectVisible}
        onOk={handleReject}
        onCancel={handleCancelReject}
        destroyOnHidden
      >
        <Form form={form} layout="vertical">
          <Form.Item
            label="驳回原因"
            name="reason"
            rules={[{ required: true, message: '请输入驳回原因' }]}
          >
            <Input.TextArea rows={4} />
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
}
