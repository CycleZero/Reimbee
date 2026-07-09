import { useState, useEffect } from 'react';
import {
  Card,
  Descriptions,
  Table,
  Button,
  Space,
  Modal,
  Form,
  InputNumber,
  Input,
  App,
  Skeleton,
  Empty,
  Result,
  Tag,
} from 'antd';
import { useParams, useNavigate } from 'react-router-dom';
import {
  getReimbursement,
  getReimbursementByNo,
  submitReimbursement,
  approveReimbursement,
  rejectReimbursement,
} from '@/api';
import { AmountText } from '@/components/common/AmountText';
import { StatusTag } from '@/components/common/StatusTag';
import { useAuthStore } from '@/stores/authStore';
import { yuanToFen, formatDate } from '@/utils/format';
import type { Reimbursement, ReimbursementItem, ReceiptItem, ApprovalInfo } from '@/types/models';

/** 核查结果颜色映射 */
const CHECK_RESULT_MAP: Record<string, { color: string; label: string }> = {
  pass: { color: 'success', label: '通过' },
  warning: { color: 'warning', label: '警告' },
  error: { color: 'error', label: '错误' },
  pending: { color: 'default', label: '待核查' },
};

/** 审批动作中文映射 */
const ACTION_LABELS: Record<string, string> = {
  pending: '待审批',
  approved: '已通过',
  rejected: '已驳回',
};

export default function ReimbursementDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const user = useAuthStore((s) => s.user);
  const { message, modal } = App.useApp();
  const [form] = Form.useForm();
  const [rejectForm] = Form.useForm();

  const [reimbursement, setReimbursement] = useState<Reimbursement | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [actionLoading, setActionLoading] = useState(false);
  const [submitVisible, setSubmitVisible] = useState(false);
  const [rejectVisible, setRejectVisible] = useState(false);

  // ---------- 数据加载 ----------

  const fetchDetail = async () => {
    if (!id) return;
    setLoading(true);
    setError(null);
    try {
      const isNumeric = /^\d+$/.test(id);
      const result = isNumeric
        ? await getReimbursement(Number(id))
        : await getReimbursementByNo(id);
      setReimbursement(result);
    } catch (err) {
      setError(err instanceof Error ? err : new Error('加载失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchDetail();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  // ---------- 权限判断 ----------

  const isOwner = user?.employee_id === reimbursement?.employee_id;
  const isApprover =
    user?.is_approver || user?.role === 'approver' || user?.role === 'admin';

  // ---------- 提交操作 ----------

  const handleSubmit = async () => {
    if (!reimbursement) return;
    try {
      const values = await form.validateFields();
      setActionLoading(true);
      await submitReimbursement(reimbursement.id, {
        total_amount: yuanToFen(values.total_amount),
      });
      message.success('已提交审批');
      setSubmitVisible(false);
      form.resetFields();
      await fetchDetail();
    } catch (err) {
      if (err instanceof Error) {
        message.error(err.message.includes('409') ? '状态不允许或预算不足' : err.message);
        // API 错误时关闭弹窗并刷新
        setSubmitVisible(false);
        form.resetFields();
        await fetchDetail();
      }
      // 表单验证失败时保持弹窗打开，Ant Design 已显示字段级错误
    } finally {
      setActionLoading(false);
    }
  };

  const handleCancelSubmit = () => {
    setSubmitVisible(false);
    form.resetFields();
  };

  // ---------- 审批操作 ----------

  const handleApprove = () => {
    if (!reimbursement) return;
    modal.confirm({
      title: '确认通过',
      content: '确认通过该报销单？',
      onOk: async () => {
        try {
          await approveReimbursement(reimbursement.id);
          message.success('已通过');
          await fetchDetail();
        } catch (err) {
          message.error(err instanceof Error ? err.message : '操作失败');
        }
      },
    });
  };

  const handleReject = async () => {
    if (!reimbursement) return;
    try {
      const values = await rejectForm.validateFields();
      setActionLoading(true);
      await rejectReimbursement(reimbursement.id, values.reason);
      message.success('已驳回');
      setRejectVisible(false);
      rejectForm.resetFields();
      await fetchDetail();
    } catch (err) {
      if (err instanceof Error) {
        message.error(err.message);
        setRejectVisible(false);
        rejectForm.resetFields();
        await fetchDetail();
      }
      // 表单验证失败时保持弹窗打开
    } finally {
      setActionLoading(false);
    }
  };

  const handleCancelReject = () => {
    setRejectVisible(false);
    rejectForm.resetFields();
  };

  // ---------- 操作按钮渲染 ----------

  const renderActions = () => {
    if (!reimbursement) return null;
    const { status } = reimbursement;

    // 草稿/已驳回 → 提交/重新提交（仅所有者）
    if ((status === 'draft' || status === 'rejected') && isOwner) {
      return (
        <Button type="primary" onClick={() => setSubmitVisible(true)} loading={actionLoading}>
          {status === 'draft' ? '提交审批' : '重新提交'}
        </Button>
      );
    }

    // 待审批/审批中 → 通过/驳回（仅审批人）
    if ((status === 'pending' || status === 'reviewing') && isApprover) {
      return (
        <Space>
          <Button type="primary" onClick={handleApprove} loading={actionLoading}>
            通过
          </Button>
          <Button danger onClick={() => setRejectVisible(true)} loading={actionLoading}>
            驳回
          </Button>
        </Space>
      );
    }

    // 已通过 → 文字标记
    if (status === 'approved') {
      return <Tag color="success">已通过</Tag>;
    }

    return null;
  };

  // ---------- 表格列定义 ----------

  const receiptColumns = [
    { title: '明细', dataIndex: 'item_desc', key: 'item' },
    { title: '类别', dataIndex: 'category', key: 'category' },
    { title: '票面金额', dataIndex: 'amount', key: 'amount', render: (v: number) => <AmountText amount={v} /> },
    { title: '日期', dataIndex: 'invoice_date', key: 'date' },
    { title: '发票号码', dataIndex: 'invoice_number', key: 'invoice' },
    { title: '核查结果', dataIndex: 'check_result', key: 'check',
      render: (v: string) => {
        const cfg = CHECK_RESULT_MAP[v] ?? { color: 'default', label: v };
        return <Tag color={cfg.color}>{cfg.label}</Tag>;
      },
    },
  ];

  const approvalColumns = [
    {
      title: '审批人',
      dataIndex: 'approver_name',
      key: 'approver',
    },
    {
      title: '动作',
      dataIndex: 'action',
      key: 'action',
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

  // ---------- 加载态 ----------

  if (loading) {
    return <Skeleton active paragraph={{ rows: 8 }} />;
  }

  // ---------- 错误态 ----------

  if (error) {
    const msg = error.message;
    if (msg.includes('不存在') || msg.includes('404')) {
      return (
        <Result
          status="404"
          title="报销单不存在"
          extra={
            <Button onClick={() => navigate('/reimbursements')}>返回列表</Button>
          }
        />
      );
    }
    // 其他错误通过 message 提示并显示空
    message.error(msg);
    return null;
  }

  // ---------- 空态 ----------

  if (!reimbursement) {
    return <Empty description="报销单不存在" />;
  }

  // ---------- 主渲染 ----------

  return (
    <>
      {/* 返回按钮 */}
      <Space style={{ marginBottom: 16 }}>
        <Button onClick={() => navigate('/reimbursements')}>← 返回列表</Button>
      </Space>

      {/* 页头 */}
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          marginBottom: 16,
        }}
      >
        <Space size="middle">
          <h2 style={{ margin: 0 }}>{reimbursement.reimbursement_no}</h2>
          <StatusTag status={reimbursement.status} />
          <span style={{ color: '#999' }}>{formatDate(reimbursement.created_at)}</span>
        </Space>
        <div>{renderActions()}</div>
      </div>

      {/* 信息卡片 */}
      <Card style={{ marginBottom: 16 }}>
        <Descriptions column={2} bordered size="small">
          <Descriptions.Item label="申请人">{reimbursement.employee_name}</Descriptions.Item>
          <Descriptions.Item label="部门">{reimbursement.department}</Descriptions.Item>
          <Descriptions.Item label="事由">
            {reimbursement.submit_note || '—'}
          </Descriptions.Item>
          <Descriptions.Item label="金额">
            <AmountText amount={reimbursement.total_amount} />
          </Descriptions.Item>
          <Descriptions.Item label="特殊审批">
            {reimbursement.need_special_approval ? '是（需特殊审批）' : '否'}
          </Descriptions.Item>
        </Descriptions>
      </Card>

      {/* 票据列表（按明细分组） */}
      <Card title="报销明细" style={{ marginBottom: 16 }}>
        <Table<ReceiptItem & { item_desc: string }>
          columns={receiptColumns}
          dataSource={reimbursement.items?.flatMap(item =>
            item.receipts?.map(rct => ({ ...rct, item_desc: `${item.category} · ${item.description || '-'}` })) ?? []
          ) ?? []}
          rowKey="id"
          size="small"
          pagination={false}
          locale={{ emptyText: <Empty description="暂无票据" /> }}
        />
      </Card>

      {/* 审批记录 */}
      <Card title="审批记录" style={{ marginBottom: 16 }}>
        <Table<ApprovalInfo>
          columns={approvalColumns}
          dataSource={reimbursement.approvals}
          rowKey="id"
          size="small"
          pagination={false}
          locale={{ emptyText: <Empty description="暂无审批记录" /> }}
        />
      </Card>

      {/* 提交审批弹窗 */}
      <Modal
        title="提交报销单"
        open={submitVisible}
        onOk={handleSubmit}
        onCancel={handleCancelSubmit}
        destroyOnHidden
        confirmLoading={actionLoading}
      >
        <Form form={form} layout="vertical">
          <Form.Item
            label="总金额（元）"
            name="total_amount"
            rules={[{ required: true, message: '请输入总金额' }]}
          >
            <InputNumber
              min={0}
              step={0.01}
              precision={2}
              addonBefore="¥"
              style={{ width: '100%' }}
              placeholder="请输入总金额"
            />
          </Form.Item>
        </Form>
      </Modal>

      {/* 驳回弹窗 */}
      <Modal
        title="驳回报销单"
        open={rejectVisible}
        onOk={handleReject}
        onCancel={handleCancelReject}
        destroyOnHidden
        confirmLoading={actionLoading}
      >
        <Form form={rejectForm} layout="vertical">
          <Form.Item
            label="驳回原因"
            name="reason"
            rules={[{ required: true, message: '请输入驳回原因' }]}
          >
            <Input.TextArea rows={4} placeholder="请输入驳回原因" />
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
}
