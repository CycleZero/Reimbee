import { Button, Table, Space, App } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { listReimbursements } from '@/api';
import { StatusTag } from '@/components/common/StatusTag';
import { AmountText } from '@/components/common/AmountText';
import { useAuthStore } from '@/stores/authStore';
import type { Reimbursement } from '@/types/models';

export default function ReimbursementList() {
  const [data, setData] = useState<Reimbursement[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(1);
  const navigate = useNavigate();
  const { message } = App.useApp();
  const user = useAuthStore((s) => s.user);

  useEffect(() => {
    (async () => {
      setLoading(true);
      try {
        const res = await listReimbursements({
          page,
          page_size: 10,
          employee_id: user?.employee_id,
        });
        setData(res.list);
        setTotal(res.total);
      } catch (err) {
        message.error(err instanceof Error ? err.message : '加载失败');
      } finally {
        setLoading(false);
      }
    })();
  }, [page, user?.employee_id, message]);

  const columns = [
    {
      title: '单号',
      dataIndex: 'reimbursement_no',
      key: 'no',
      render: (v: string) => (
        <a onClick={() => navigate(`/reimbursements/${v}`)}>{v}</a>
      ),
    },
    { title: '申请人', dataIndex: 'employee_name', key: 'name' },
    {
      title: '金额',
      dataIndex: 'total_amount',
      key: 'amount',
      render: (v: number) => <AmountText amount={v} />,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (v: string) => <StatusTag status={v} />,
    },
    { title: '事由', dataIndex: 'submit_note', key: 'note', ellipsis: true },
  ];

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => navigate('/reimbursements/create')}>
          新建报销
        </Button>
      </Space>
      <Table
        columns={columns}
        dataSource={data}
        rowKey="id"
        loading={loading}
        pagination={{ current: page, total, pageSize: 10, onChange: setPage }}
        onRow={(r) => ({ onClick: () => navigate(`/reimbursements/${r.id}`), style: { cursor: 'pointer' } })}
      />
    </>
  );
}
