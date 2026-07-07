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

  const { summary, departments } = data;

  const columns = [
    { title: '部门', dataIndex: 'department', key: 'name' },
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
      title: '冻结',
      dataIndex: 'frozen_amount',
      key: 'frozen',
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
      key: 'rate',
      render: (v: number) => `${(v * 100).toFixed(1)}%`,
    },
  ];

  return (
    <>
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
            <Statistic
              title="总使用率"
              value={summary.overall_usage}
              suffix="%"
              precision={1}
              formatter={(v) => ((v as number) * 100).toFixed(1)}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="剩余可用" value={formatAmount(summary.total_remaining)} />
          </Card>
        </Col>
      </Row>

      <Card title="各部门预算详情">
        <Table
          columns={columns}
          dataSource={departments}
          rowKey="id"
          pagination={false}
        />
      </Card>
    </>
  );
}
