import { Button, Card, Form, Input, Typography, App } from 'antd';
import { UserOutlined, LockOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { login } from '@/api';
import { useAuthStore } from '@/stores/authStore';
import type { LoginRequest } from '@/types/models';

export default function Login() {
  const [form] = Form.useForm<LoginRequest>();
  const { message } = App.useApp();
  const navigate = useNavigate();
  const authLogin = useAuthStore((s) => s.login);

  const handleSubmit = async (values: LoginRequest) => {
    try {
      const res = await login(values);
      authLogin(res.token, res.employee);
      message.success(`欢迎回来，${res.employee.name}`);
      navigate('/dashboard', { replace: true });
    } catch (err) {
      message.error(err instanceof Error ? err.message : '登录失败');
    }
  };

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
      }}
    >
      <Card style={{ width: 400, borderRadius: 16 }}>
        <Typography.Title level={3} style={{ textAlign: 'center', marginBottom: 32 }}>
          🏦 Reimbee
        </Typography.Title>
        <Typography.Text type="secondary" style={{ display: 'block', textAlign: 'center', marginBottom: 24 }}>
          企业财务报销助手
        </Typography.Text>

        <Form form={form} onFinish={handleSubmit} size="large" layout="vertical">
          <Form.Item
            name="employee_id"
            rules={[{ required: true, message: '请输入工号' }]}
          >
            <Input prefix={<UserOutlined />} placeholder="工号" />
          </Form.Item>
          <Form.Item
            name="password"
            rules={[{ required: true, message: '请输入密码' }]}
          >
            <Input.Password prefix={<LockOutlined />} placeholder="密码" />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" block>
              登 录
            </Button>
          </Form.Item>
        </Form>
      </Card>
    </div>
  );
}
