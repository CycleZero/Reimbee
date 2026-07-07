import { Card, Form, Input, Select, Button, App, Skeleton, Space, Upload } from 'antd';
import { ArrowLeftOutlined, UploadOutlined, DeleteOutlined } from '@ant-design/icons';
import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { createReimbursement, listDepartments, uploadInvoice } from '@/api';
import { useAuthStore } from '@/stores/authStore';
import type { Department, UploadInvoiceResponse } from '@/types/models';

export default function ReimbursementCreate() {
  const [departments, setDepartments] = useState<Department[]>([]);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [uploadedFiles, setUploadedFiles] = useState<UploadInvoiceResponse[]>([]);
  const [uploadLoading, setUploadLoading] = useState(false);

  const [form] = Form.useForm();
  const navigate = useNavigate();
  const { message } = App.useApp();
  const user = useAuthStore((s) => s.user);

  // 加载部门列表
  useEffect(() => {
    (async () => {
      setLoading(true);
      try {
        const res = await listDepartments({ page_size: 100 });
        setDepartments(res.list);
      } catch (err) {
        message.error(err instanceof Error ? err.message : '加载部门列表失败');
      } finally {
        setLoading(false);
      }
    })();
  }, [message]);

  // 上传票据文件
  const handleUpload = async (file: File) => {
    setUploadLoading(true);
    try {
      const res = await uploadInvoice(file);
      setUploadedFiles((prev) => [...prev, res]);
      message.success(`${file.name} 上传成功`);
    } catch (err) {
      message.error(err instanceof Error ? err.message : '文件上传失败');
    } finally {
      setUploadLoading(false);
    }
  };

  // 删除已上传文件
  const handleRemoveFile = (fileId: string) => {
    setUploadedFiles((prev) => prev.filter((f) => f.file_id !== fileId));
  };

  // 提交报销单
  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null);
    if (!values) return; // 表单验证失败，antd 已展示错误信息

    if (!user) {
      message.error('用户信息缺失，请重新登录');
      return;
    }

    setSubmitting(true);
    try {
      const req = {
        employee_id: user.employee_id,
        employee_name: user.name,
        department_id: values.department_id,
        submit_note: values.submit_note || undefined,
      };
      const res = await createReimbursement(req);
      message.success('草稿已创建');
      navigate(`/reimbursements/${res.id}`);
    } catch (err) {
      message.error(err instanceof Error ? err.message : '创建失败');
    } finally {
      setSubmitting(false);
    }
  };

  if (loading) {
    return <Skeleton active paragraph={{ rows: 6 }} />;
  }

  return (
    <Card
      title="创建报销单"
      extra={
        <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/reimbursements')}>
          返回
        </Button>
      }
    >
      <Space direction="vertical" size="large" style={{ width: '100%' }}>
        {/* 员工信息（只读） */}
        <div style={{ display: 'flex', gap: 16 }}>
          <div style={{ flex: 1 }}>
            <div style={{ marginBottom: 8, color: 'rgba(0,0,0,0.45)' }}>工号</div>
            <Input value={user?.employee_id ?? ''} disabled />
          </div>
          <div style={{ flex: 1 }}>
            <div style={{ marginBottom: 8, color: 'rgba(0,0,0,0.45)' }}>姓名</div>
            <Input value={user?.name ?? ''} disabled />
          </div>
        </div>

        {/* 报销表单 */}
        <Form form={form} layout="vertical">
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
          <Form.Item name="submit_note" label="事由说明">
            <Input.TextArea rows={3} placeholder="请输入报销事由..." />
          </Form.Item>
        </Form>

        {/* 票据上传区域 */}
        <div>
          <div style={{ marginBottom: 8, fontWeight: 500 }}>票据上传</div>
          <Upload
            accept="image/jpeg,image/png,image/bmp,image/tiff,application/pdf"
            showUploadList={false}
            beforeUpload={(file) => {
              const isLt10M = file.size / 1024 / 1024 < 10;
              if (!isLt10M) {
                message.error('文件大小不能超过 10MB');
                return Upload.LIST_IGNORE;
              }
              handleUpload(file);
              return false;
            }}
          >
            <Button icon={<UploadOutlined />} loading={uploadLoading}>
              选择文件
            </Button>
          </Upload>

          {uploadedFiles.length > 0 && (
            <div style={{ marginTop: 12 }}>
              {uploadedFiles.map((f) => (
                <div
                  key={f.file_id}
                  style={{ display: 'flex', alignItems: 'center', marginBottom: 8 }}
                >
                  <span style={{ flex: 1 }}>{f.file_name}</span>
                  <Button
                    type="text"
                    danger
                    icon={<DeleteOutlined />}
                    onClick={() => handleRemoveFile(f.file_id)}
                    aria-label={`删除 ${f.file_name}`}
                  />
                </div>
              ))}
            </div>
          )}
        </div>

        {/* 提交按钮 */}
        <Button type="primary" onClick={handleSubmit} loading={submitting}>
          保存草稿
        </Button>
      </Space>
    </Card>
  );
}
