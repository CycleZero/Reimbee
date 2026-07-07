import { Button, Table, Space, Modal, Form, Input, Select, App, Card } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import { useEffect, useState, useCallback } from 'react';
import {
  listDepartments,
  createDepartment,
  updateDepartment,
  deleteDepartment,
  listApprovers,
} from '@/api';
import type { Department, Employee } from '@/types/models';

export default function DepartmentList() {
  const [data, setData] = useState<Department[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(1);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingDept, setEditingDept] = useState<Department | null>(null);
  const [approvers, setApprovers] = useState<Employee[]>([]);
  const [form] = Form.useForm();
  const { message, modal } = App.useApp();

  // 获取部门列表
  const fetchDepartments = useCallback(async () => {
    setLoading(true);
    try {
      const res = await listDepartments({ page, page_size: 10 });
      setData(res.list);
      setTotal(res.total);
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载部门列表失败');
    } finally {
      setLoading(false);
    }
  }, [page, message]);

  useEffect(() => {
    fetchDepartments();
  }, [fetchDepartments]);

  // 获取审批人列表，供负责人下拉框使用
  useEffect(() => {
    (async () => {
      try {
        const list = await listApprovers();
        setApprovers(list);
      } catch (err) {
        message.error(err instanceof Error ? err.message : '加载审批人列表失败');
      }
    })();
  }, [message]);

  // 打开新建/编辑弹窗
  const openModal = (dept?: Department) => {
    if (dept) {
      setEditingDept(dept);
    } else {
      form.resetFields();
      setEditingDept(null);
    }
    setModalVisible(true);
  };

  // 编辑弹窗打开时预填表单数据
  useEffect(() => {
    if (!modalVisible) return;
    if (editingDept) {
      form.setFieldsValue({ name: editingDept.name, manager_id: editingDept.manager_id ?? undefined });
    }
  }, [modalVisible, editingDept, form]);

  // 关闭弹窗
  const closeModal = () => {
    setModalVisible(false);
    setEditingDept(null);
    form.resetFields();
  };

  // 提交表单（新建 / 编辑）
  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      if (editingDept) {
        await updateDepartment(editingDept.id, values);
        message.success('部门信息已更新');
      } else {
        await createDepartment(values);
        message.success('部门已创建');
      }
      closeModal();
      fetchDepartments();
    } catch (err) {
      if (err instanceof Error) message.error(err.message);
    }
  };

  // 删除部门
  const handleDelete = (dept: Department) => {
    modal.confirm({
      title: '确认删除',
      content: `确认删除部门「${dept.name}」？`,
      okText: '删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        try {
          await deleteDepartment(dept.id);
          message.success('部门已删除');
          if (data.length === 1 && page > 1) {
            setPage(page - 1);
          } else {
            fetchDepartments();
          }
        } catch (err) {
          message.error(err instanceof Error ? err.message : '删除失败');
        }
      },
    });
  };

  // 按审批人 ID 查找姓名
  const getApproverName = (managerId?: number) => {
    if (!managerId) return '—';
    const approver = approvers.find((a) => a.id === managerId);
    return approver?.name ?? '—';
  };

  // 表格列定义
  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      width: 80,
    },
    {
      title: '部门名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '负责人',
      dataIndex: 'manager_id',
      key: 'manager_id',
      render: (_: unknown, record: Department) => getApproverName(record.manager_id),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
    },
    {
      title: '操作',
      key: 'action',
      render: (_: unknown, record: Department) => (
        <Space>
          <a onClick={() => openModal(record)}>编辑</a>
          <a style={{ color: '#ff4d4f' }} onClick={() => handleDelete(record)}>
            删除
          </a>
        </Space>
      ),
    },
  ];

  return (
    <Card title="部门管理">
      <Space style={{ marginBottom: 16 }}>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => openModal()}>
          新建部门
        </Button>
      </Space>

      <Table
        columns={columns}
        dataSource={data}
        rowKey="id"
        loading={loading}
        pagination={{ current: page, total, pageSize: 10, onChange: setPage }}
      />

      <Modal
        title={editingDept ? '编辑部门' : '新建部门'}
        open={modalVisible}
        onOk={handleSubmit}
        onCancel={closeModal}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item
            label="部门名称"
            name="name"
            rules={[{ required: true, message: '请输入部门名称' }]}
          >
            <Input placeholder="请输入部门名称" />
          </Form.Item>

          <Form.Item label="负责人" name="manager_id">
            <Select
              allowClear
              placeholder="请选择负责人（可选）"
              options={approvers.map((a) => ({
                label: a.name,
                value: a.id,
              }))}
            />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
}
