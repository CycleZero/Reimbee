import { Card, Statistic, Row, Col, Table, Skeleton, App } from 'antd';
import { useEffect, useState } from 'react';
import { getBudgetDashboard } from '@/api';
import { formatAmount } from '@/utils/format';
import type { BudgetDashboard } from '@/types/models';

export default function Dashboard() {
  const [data, setData] = useState<BudgetDashboard | null>(null);
  const [loading, setLoading] = useState(true);
  const { message } = App.useApp();

  useEffect(() => {
    (async () => {
      try {
        const res = await getBudgetDashboard();
        setData(res);
      } catch (err) {
        message.error(err instanceof Error ? err.message : '加载失败');
      } finally {
        setLoading(false);
      }
    })();
  }, [message]);

  if (loading) return <Skeleton active paragraph={{ rows: 8 }} />;
  if (!data) return null;

  const totalBudget = data.departments.reduce((s, d) => s + d.annual_budget, 0);
  const totalSpent = data.departments.reduce((s, d) => s + d.spent_amount, 0);
  const totalFrozen = data.departments.reduce((s, d) => s + d.frozen_amount, 0);
  const totalRemaining = totalBudget - totalSpent - totalFrozen;

  const columns = [
    { title: '部门', dataIndex: 'department_name', key: 'name' },
    {
      title: '年度预算',
      dataIndex: 'annual_budget',
      key: 'budget',
      render: (v: number) => formatAmount(v),
    },
    {
      title: '已支出',
      dataIndex: 'spent_amount',
      key: 'spent',
      render: (v: number) => formatAmount(v),
    },
    {
      title: '剩余',
      key: 'remaining',
      render: (_: unknown, r: (typeof data.departments)[number]) =>
        formatAmount(r.annual_budget - r.spent_amount - r.frozen_amount),
    },
    {
      title: '使用率',
      dataIndex: 'usage_rate',
      key: 'rate',
      render: (v: number) => `${(v * 100).toFixed(1)}%`,
    },
  ];

  return (
    <>
      <Row gutter={16} style={{ marginBottom: 24 }}>
        <Col span={6}>
          <Card><Statistic title="总预算" value={formatAmount(totalBudget)} /></Card>
        </Col>
        <Col span={6}>
          <Card><Statistic title="已支出" value={formatAmount(totalSpent)} /></Card>
        </Col>
        <Col span={6}>
          <Card><Statistic title="冻结金额" value={formatAmount(totalFrozen)} /></Card>
        </Col>
        <Col span={6}>
          <Card><Statistic title="剩余可用" value={formatAmount(totalRemaining)} /></Card>
        </Col>
      </Row>

      <Card title={`${data.year} 财年 — 各部门预算详情`}>
        <Table
          columns={columns}
          dataSource={data.departments}
          rowKey="id"
          pagination={false}
        />
      </Card>
    </>
  );
}
