import {
  Card,
  Statistic,
  Row,
  Col,
  Table,
  Button,
  Space,
  Modal,
  Form,
  InputNumber,
  Select,
  Skeleton,
  App,
} from 'antd';
import { useEffect, useState } from 'react';
import {
  getBudgetDashboard,
  createBudget,
  updateBudget,
  listDepartments,
} from '@/api';
import { formatAmount, yuanToFen } from '@/utils/format';
import type { BudgetDashboard, DepartmentBudget, Department } from '@/types/models';

export default function BudgetManage() {
  // ---------- 状态 ----------
  const [dashboardData, setDashboardData] = useState<BudgetDashboard | null>(null);
  const [loading, setLoading] = useState(true);
  const [fiscalYear, setFiscalYear] = useState<number>(new Date().getFullYear());
  const [createVisible, setCreateVisible] = useState(false);
  const [editVisible, setEditVisible] = useState(false);
  const [editingBudget, setEditingBudget] = useState<DepartmentBudget | null>(null);
  const [departments, setDepartments] = useState<Department[]>([]);
  const { message } = App.useApp();

  const [createForm] = Form.useForm();
  const [editForm] = Form.useForm();

  // ---------- 加载数据 ----------

  /** 加载预算看板 */
  const fetchDashboard = async () => {
    try {
      const res = await getBudgetDashboard(fiscalYear);
      setDashboardData(res);
    } catch (err) {
      message.error(err instanceof Error ? err.message : '刷新预算数据失败');
    }
  };

  /** 切换财年 / 首次挂载：拉取看板 */
  useEffect(() => {
    (async () => {
      setLoading(true);
      try {
        const res = await getBudgetDashboard(fiscalYear);
        setDashboardData(res);
      } catch (err) {
        message.error(err instanceof Error ? err.message : '加载预算数据失败');
      } finally {
        setLoading(false);
      }
    })();
  }, [fiscalYear, message]);

  /** 拉取部门列表（供 Select 下拉） */
  useEffect(() => {
    (async () => {
      try {
        const res = await listDepartments({ page_size: 999 });
        setDepartments(res.list);
      } catch (err) {
        message.error(err instanceof Error ? err.message : '加载部门列表失败');
      }
    })();
  }, [message]);

  /** 编辑弹窗打开时，把选中预算金额（分→元）填入表单 */
  useEffect(() => {
    if (editVisible && editingBudget) {
      editForm.setFieldsValue({ annual_budget: editingBudget.annual_budget / 100 });
    }
  }, [editVisible, editingBudget, editForm]);

  // ---------- 加载态 / 空态 ----------
  if (loading) return <Skeleton active paragraph={{ rows: 8 }} />;
  if (!dashboardData) return null;

  const { summary, departments: budgetDepartments } = dashboardData;

  // ---------- 表格列定义 ----------

  const columns = [
    { title: '部门', dataIndex: 'department', key: 'department' },
    { title: '财年', dataIndex: 'fiscal_year', key: 'fiscal_year' },
    {
      title: '年度预算',
      dataIndex: 'annual_budget',
      key: 'annual_budget',
      render: (v: number) => formatAmount(v),
    },
    {
      title: '已支出',
      dataIndex: 'spent_amount',
      key: 'spent_amount',
      render: (v: number) => formatAmount(v),
    },
    {
      title: '冻结',
      dataIndex: 'frozen_amount',
      key: 'frozen_amount',
      render: (v: number) => formatAmount(v),
    },
    {
      title: '剩余',
      dataIndex: 'remaining',
      key: 'remaining',
      render: (v: number) => formatAmount(v),
    },
    {
      title: '使用率',
      dataIndex: 'usage_rate',
      key: 'usage_rate',
      render: (v: number) => `${(v * 100).toFixed(1)}%`,
    },
    {
      title: '操作',
      key: 'action',
      render: (_: unknown, record: DepartmentBudget) => (
        <Button
          type="link"
          onClick={() => {
            setEditingBudget(record);
            setEditVisible(true);
          }}
        >
          编辑
        </Button>
      ),
    },
  ];

  // ---------- 创建预算 ----------

  const handleCreate = async () => {
    try {
      const values = await createForm.validateFields();
      const data = {
        ...values,
        annual_budget: yuanToFen(values.annual_budget),
      };
      await createBudget(data);
      message.success('预算创建成功');
      setCreateVisible(false);
      createForm.resetFields();
      await fetchDashboard();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '创建预算失败');
    }
  };

  // ---------- 编辑预算 ----------

  const handleEdit = async () => {
    if (!editingBudget) return;
    try {
      const values = await editForm.validateFields();
      await updateBudget(editingBudget.id, {
        annual_budget: yuanToFen(values.annual_budget),
      });
      message.success('预算更新成功');
      setEditVisible(false);
      setEditingBudget(null);
      editForm.resetFields();
      await fetchDashboard();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '更新预算失败');
    }
  };

  // ---------- 渲染 ----------

  return (
    <>
      {/* 财年选择 */}
      <Select
        value={fiscalYear}
        onChange={setFiscalYear}
        options={[2024, 2025, 2026, 2027].map((y) => ({
          label: `${y}年`,
          value: y,
        }))}
        style={{ width: 120, marginBottom: 24 }}
      />

      {/* 汇总卡片 */}
      <Row gutter={16} style={{ marginBottom: 24 }}>
        <Col span={6}>
          <Card>
            <Statistic title="总预算" value={formatAmount(summary.total_budget)} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="已支出" value={formatAmount(summary.total_spent)} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="剩余" value={formatAmount(summary.total_remaining)} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="总使用率"
              value={summary.overall_usage}
              suffix="%"
              precision={1}
              formatter={(v) => ((v as number) * 100).toFixed(1)}
            />
          </Card>
        </Col>
      </Row>

      {/* 各部门预算详情表格 */}
      <Card
        title="各部门预算详情"
        extra={
          <Button type="primary" onClick={() => setCreateVisible(true)}>
            添加预算
          </Button>
        }
      >
        <Table
          columns={columns}
          dataSource={budgetDepartments}
          rowKey="id"
          pagination={false}
        />
      </Card>

      {/* 创建预算 Modal */}
      <Modal
        title="添加预算"
        open={createVisible}
        onOk={handleCreate}
        onCancel={() => {
          setCreateVisible(false);
          createForm.resetFields();
        }}
        destroyOnHidden
      >
        <Form form={createForm} layout="vertical">
          <Form.Item
            name="department_id"
            label="部门"
            rules={[{ required: true, message: '请选择部门' }]}
          >
            <Select
              options={departments.map((d) => ({
                label: d.name,
                value: d.id,
              }))}
              placeholder="请选择部门"
            />
          </Form.Item>
          <Form.Item
            name="fiscal_year"
            label="财年"
            rules={[{ required: true, message: '请输入财年' }]}
          >
            <InputNumber min={2020} max={2099} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item
            name="annual_budget"
            label="年度预算(元)"
            rules={[{ required: true, message: '请输入年度预算' }]}
          >
            <Space.Compact style={{ width: '100%' }}>
              <Button disabled>¥</Button>
              <InputNumber
                min={0}
                step={0.01}
                precision={2}
                style={{ flex: 1 }}
              />
            </Space.Compact>
          </Form.Item>
        </Form>
      </Modal>

      {/* 编辑预算 Modal */}
      <Modal
        title="编辑预算"
        open={editVisible}
        onOk={handleEdit}
        onCancel={() => {
          setEditVisible(false);
          setEditingBudget(null);
          editForm.resetFields();
        }}
        destroyOnHidden
      >
        <Form form={editForm} layout="vertical">
          <Form.Item
            name="annual_budget"
            label="年度预算(元)"
            rules={[{ required: true, message: '请输入年度预算' }]}
          >
            <Space.Compact style={{ width: '100%' }}>
              <Button disabled>¥</Button>
              <InputNumber
                min={0}
                step={0.01}
                precision={2}
                style={{ flex: 1 }}
              />
            </Space.Compact>
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
}
