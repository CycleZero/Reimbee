import { useState, useEffect } from 'react';
import { Button, Card, Form, Input, Typography, App, Select, Tabs } from 'antd';
import { UserOutlined, LockOutlined, MailOutlined, BankOutlined, IdcardOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { login, register } from '@/api';
import { api } from '@/api/client';
import { useAuthStore } from '@/stores/authStore';
import type { LoginRequest, Department, PaginatedResponse } from '@/types/models';

export default function Login() {
  const [loginForm] = Form.useForm<LoginRequest>();
  const [registerForm] = Form.useForm();
  const [activeTab, setActiveTab] = useState<'login' | 'register'>('login');
  const [departments, setDepartments] = useState<Department[]>([]);
  const [registering, setRegistering] = useState(false);
  const { message } = App.useApp();
  const navigate = useNavigate();
  const authLogin = useAuthStore((s) => s.login);

  // 注册时加载部门列表
  useEffect(() => {
    if (activeTab === 'register') {
      api.get<PaginatedResponse<Department>>('/api/departments', { public: true })
        .then((res) => setDepartments(res.list))
        .catch(() => message.error('加载部门列表失败'));
    }
  }, [activeTab, message]);

  const handleLogin = async (values: LoginRequest) => {
    try {
      const res = await login(values);
      authLogin(res.token, {
        employee_id: res.employee_id,
        name: res.name,
        role: res.role,
      });
      message.success(`欢迎回来，${res.name}`);
      navigate('/dashboard', { replace: true });
    } catch (err) {
      message.error(err instanceof Error ? err.message : '登录失败');
    }
  };

  const handleRegister = async (values: {
    name: string;
    department_id: number;
    password: string;
    confirm_password: string;
    email?: string;
  }) => {
    if (values.password !== values.confirm_password) {
      message.error('两次密码不一致');
      return;
    }
    setRegistering(true);
    try {
      const res = await register({
        name: values.name,
        department_id: values.department_id,
        password: values.password,
        email: values.email,
      });
      message.success(res.message || '注册成功，请登录');
      registerForm.resetFields();
      setActiveTab('login');
    } catch (err) {
      message.error(err instanceof Error ? err.message : '注册失败');
    } finally {
      setRegistering(false);
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
      <Card style={{ width: 420, borderRadius: 16 }}>
        <Typography.Title level={3} style={{ textAlign: 'center', marginBottom: 8 }}>
          🏦 Reimbee
        </Typography.Title>
        <Typography.Text type="secondary" style={{ display: 'block', textAlign: 'center', marginBottom: 24 }}>
          企业财务报销助手
        </Typography.Text>

        <Tabs
          activeKey={activeTab}
          onChange={(k) => setActiveTab(k as 'login' | 'register')}
          centered
          items={[
            {
              key: 'login',
              label: '登录',
              children: (
                <Form form={loginForm} onFinish={handleLogin} size="large" layout="vertical">
                  <Form.Item name="employee_id" rules={[{ required: true, message: '请输入工号' }]}>
                    <Input prefix={<IdcardOutlined />} placeholder="工号" />
                  </Form.Item>
                  <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
                    <Input.Password prefix={<LockOutlined />} placeholder="密码" />
                  </Form.Item>
                  <Form.Item>
                    <Button type="primary" htmlType="submit" block>
                      登 录
                    </Button>
                  </Form.Item>
                </Form>
              ),
            },
            {
              key: 'register',
              label: '注册',
              children: (
                <Form form={registerForm} onFinish={handleRegister} size="large" layout="vertical">
                  <Form.Item name="name" rules={[{ required: true, message: '请输入姓名' }]}>
                    <Input prefix={<UserOutlined />} placeholder="姓名" />
                  </Form.Item>
                  <Form.Item name="department_id" rules={[{ required: true, message: '请选择部门' }]}>
                    <Select
                      prefix={<BankOutlined />}
                      placeholder="选择部门"
                      options={departments.map((d) => ({ label: d.name, value: d.id }))}
                      showSearch
                      optionFilterProp="label"
                      notFoundContent="暂无部门数据"
                    />
                  </Form.Item>
                  <Form.Item name="email" rules={[{ type: 'email', message: '邮箱格式不正确' }]}>
                    <Input prefix={<MailOutlined />} placeholder="邮箱（选填）" />
                  </Form.Item>
                  <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }, { min: 6, message: '密码至少6位' }]}>
                    <Input.Password prefix={<LockOutlined />} placeholder="密码" />
                  </Form.Item>
                  <Form.Item
                    name="confirm_password"
                    dependencies={['password']}
                    rules={[
                      { required: true, message: '请确认密码' },
                      ({ getFieldValue }) => ({
                        validator(_, value) {
                          if (!value || getFieldValue('password') === value) return Promise.resolve();
                          return Promise.reject(new Error('两次密码不一致'));
                        },
                      }),
                    ]}
                  >
                    <Input.Password prefix={<LockOutlined />} placeholder="确认密码" />
                  </Form.Item>
                  <Form.Item>
                    <Button type="primary" htmlType="submit" block loading={registering}>
                      注 册
                    </Button>
                  </Form.Item>
                </Form>
              ),
            },
          ]}
        />
      </Card>
    </div>
  );
}
