import { Button, Table, Space, App, Modal, Form, Input, Select } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import { useEffect, useState, useCallback } from 'react';
import { listEmployees, createEmployee, updateEmployee, deleteEmployee, listDepartments } from '@/api';
import { ROLE_LABELS } from '@/utils/constants';
import type { Employee, CreateEmployeeRequest, Department } from '@/types/models';

export default function EmployeeList() {
  const [data, setData] = useState<Employee[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(1);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingEmployee, setEditingEmployee] = useState<Employee | null>(null);
  const [departments, setDepartments] = useState<Department[]>([]);
  const { message, modal } = App.useApp();
  const [form] = Form.useForm();

  // 分页获取员工列表
  const fetchEmployees = useCallback(async () => {
    setLoading(true);
    try {
      const res = await listEmployees({ page, page_size: 10 });
      setData(res.list);
      setTotal(res.total);
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载失败');
    } finally {
      setLoading(false);
    }
  }, [page, message]);

  useEffect(() => {
    fetchEmployees();
  }, [fetchEmployees]);

  // 获取部门列表（用于下拉选择）
  useEffect(() => {
    (async () => {
      try {
        const res = await listDepartments({ page_size: 100 });
        setDepartments(res.list);
      } catch {
        // 部门加载失败不阻塞主流程
      }
    })();
  }, []);

  // 编辑弹窗打开时预填表单数据
  useEffect(() => {
    if (!modalVisible) return;
    if (editingEmployee) {
      form.setFieldsValue({
        name: editingEmployee.name,
        department_id: editingEmployee.department_id,
        email: editingEmployee.email,
        role: editingEmployee.role,
      });
    }
  }, [modalVisible, editingEmployee, form]);

  // 打开新建/编辑弹窗
  const openModal = (employee: Employee | null = null) => {
    setEditingEmployee(employee);
    setModalVisible(true);
  };

  // 关闭弹窗
  const handleModalClose = () => {
    setModalVisible(false);
    setEditingEmployee(null);
  };

  // 删除员工
  const handleDelete = (employee: Employee) => {
    modal.confirm({
      title: '确认删除该员工？',
      content: `确定要删除员工「${employee.name}（${employee.employee_id}）」吗？此操作不可恢复。`,
      okText: '确认删除',
      cancelText: '取消',
      okButtonProps: { danger: true },
      onOk: async () => {
        try {
          await deleteEmployee(employee.id);
          message.success('删除成功');
          fetchEmployees();
        } catch (err) {
          message.error(err instanceof Error ? err.message : '删除失败');
        }
      },
    });
  };

  // 提交表单（创建/更新）
  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      if (editingEmployee) {
        await updateEmployee(editingEmployee.id, values);
        message.success('更新成功');
      } else {
        await createEmployee(values as CreateEmployeeRequest);
        message.success('创建成功');
      }
      handleModalClose();
    } catch (err) {
      // validateFields 验证失败不弹 message（表单已有行内错误提示）
      if (err instanceof Error) {
        message.error(err.message);
      }
    }
  };

  const columns = [
    { title: '姓名', dataIndex: 'name', key: 'name' },
    { title: '工号', dataIndex: 'employee_id', key: 'employee_id' },
    { title: '部门', dataIndex: 'department', key: 'department' },
    { title: '邮箱', dataIndex: 'email', key: 'email' },
    {
      title: '角色',
      dataIndex: 'role',
      key: 'role',
      render: (v: string) => ROLE_LABELS[v] ?? v,
    },
    {
      title: '审批人',
      dataIndex: 'is_approver',
      key: 'is_approver',
      render: (v: boolean) => (v ? '是' : '否'),
    },
    { title: '创建时间', dataIndex: 'created_at', key: 'created_at' },
    {
      title: '操作',
      key: 'action',
      render: (_: unknown, record: Employee) => (
        <Space>
          <Button type="link" onClick={() => openModal(record)}>
            编辑
          </Button>
          <Button type="link" danger onClick={() => handleDelete(record)}>
            删除
          </Button>
        </Space>
      ),
    },
  ];

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => openModal()}>
          新建员工
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
        title={editingEmployee ? '编辑员工' : '新建员工'}
        open={modalVisible}
        onOk={handleSubmit}
        onCancel={handleModalClose}
        afterClose={() => form.resetFields()}
        destroyOnHidden={false}
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          {/* 工号：仅新建时显示 */}
          {!editingEmployee && (
            <Form.Item
              name="employee_id"
              label="工号"
              rules={[
                { required: true, message: '请输入工号' },
                { pattern: /^EMP\d+$/, message: '工号格式为 EMP 后接数字（如 EMP001）' },
              ]}
            >
              <Input placeholder="如 EMP001" />
            </Form.Item>
          )}
          <Form.Item
            name="name"
            label="姓名"
            rules={[{ required: true, message: '请输入姓名' }]}
          >
            <Input />
          </Form.Item>
          <Form.Item
            name="department_id"
            label="部门"
            rules={[{ required: true, message: '请选择部门' }]}
          >
            <Select
              placeholder="请选择部门"
              options={departments.map((d) => ({ label: d.name, value: d.id }))}
            />
          </Form.Item>
          <Form.Item
            name="email"
            label="邮箱"
            rules={[{ type: 'email', message: '请输入有效的邮箱地址' }]}
          >
            <Input placeholder="选填" />
          </Form.Item>
          <Form.Item name="role" label="角色" initialValue="employee">
            <Select
              options={[
                { label: '员工', value: 'employee' },
                { label: '审批人', value: 'approver' },
                { label: '管理员', value: 'admin' },
              ]}
            />
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
}
