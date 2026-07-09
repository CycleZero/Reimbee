import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import { Button, Layout, Avatar, Dropdown } from 'antd';
import {
  DashboardOutlined,
  FileTextOutlined,
  CheckCircleOutlined,
  TeamOutlined,
  BankOutlined,
  DollarOutlined,
  RobotOutlined,
  BookOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
} from '@ant-design/icons';
import { useAuthStore } from '@/stores/authStore';
import { useAppStore } from '@/stores/appStore';
import { AuthGuard } from './AuthGuard';
import { ROLE_LABELS } from '@/utils/constants';
import { useMemo } from 'react';

const { Header, Sider, Content } = Layout;

const MENU_ITEMS = [
  { key: '/dashboard', icon: <DashboardOutlined />, label: '仪表盘' },
  { key: '/reimbursements', icon: <FileTextOutlined />, label: '报销管理' },
  { key: '/reimbursements/pending', icon: <CheckCircleOutlined />, label: '待审批', roles: ['approver', 'admin'] },
  { key: '/employees', icon: <TeamOutlined />, label: '员工管理', roles: ['approver', 'admin'] },
  { key: '/departments', icon: <BankOutlined />, label: '部门管理', roles: ['admin'] },
  { key: '/budgets', icon: <DollarOutlined />, label: '预算管理', roles: ['admin'] },
  { key: '/policies', icon: <BookOutlined />, label: '知识库', roles: ['admin'] },
  { key: '/chat', icon: <RobotOutlined />, label: 'AI 助手' },
];

function AppLayoutInner() {
  const navigate = useNavigate();
  const location = useLocation();
  const { user, logout } = useAuthStore();
  const { sidebarCollapsed, toggleSidebar } = useAppStore();

  const filteredMenu = useMemo(
    () => MENU_ITEMS.filter((item) => !item.roles || item.roles.includes(user!.role)),
    [user],
  );

  return (
    <Layout style={{ minHeight: '100vh' }}>
      {/* 侧边栏 */}
      <Sider
        trigger={null}
        collapsible
        collapsed={sidebarCollapsed}
        theme="light"
        width={240}
        style={{ borderRight: '1px solid #F0F0F0' }}
      >
        <div style={{
          height: 64,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          borderBottom: '1px solid #F0F0F0',
          marginBottom: 8,
        }}>
          <h2 style={{ margin: 0, fontSize: sidebarCollapsed ? 18 : 20, whiteSpace: 'nowrap' }}>
            🏦 {!sidebarCollapsed && 'Reimbee'}
          </h2>
        </div>

        <div style={{ padding: '0 8px' }}>
          {filteredMenu.map((item) => {
            const active = location.pathname.startsWith(item.key);
            return (
              <div
                key={item.key}
                onClick={() => navigate(item.key)}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 12,
                  padding: '10px 16px',
                  margin: '2px 0',
                  borderRadius: 90,
                  cursor: 'pointer',
                  fontSize: 14,
                  color: active ? '#1677FF' : '#333',
                  background: active ? '#E6F4FF' : 'transparent',
                  fontWeight: active ? 600 : 400,
                  transition: 'all 0.2s',
                }}
              >
                <span style={{ fontSize: 16, width: 22, textAlign: 'center', flexShrink: 0 }}>
                  {item.icon}
                </span>
                {!sidebarCollapsed && <span>{item.label}</span>}
              </div>
            );
          })}
        </div>
      </Sider>

      {/* 右侧主区域 */}
      <Layout>
        <Header style={{
          background: '#F5F5F5',
          padding: '0 24px',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          height: 64,
          borderBottom: '1px solid #F0F0F0',
        }}>
          <Button
            type="text"
            icon={sidebarCollapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
            onClick={toggleSidebar}
            style={{ fontSize: 16, width: 40, height: 40 }}
          />

          <Dropdown
            menu={{
              items: [
                { key: 'logout', icon: <LogoutOutlined />, label: '退出登录', danger: true },
              ],
              onClick: ({ key }) => {
                if (key === 'logout') { logout(); navigate('/login'); }
              },
            }}
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
              <Avatar style={{ backgroundColor: '#1677FF' }}>{user!.name?.[0]}</Avatar>
              <span style={{ fontSize: 14 }}>{user!.name}（{ROLE_LABELS[user!.role] ?? user!.role}）</span>
            </div>
          </Dropdown>
        </Header>

        <Content style={{
          margin: 24,
          padding: 24,
          background: '#FFFFFF',
          borderRadius: 12,
          minHeight: 280,
        }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
}

export function AppLayout() {
  return (
    <AuthGuard>
      <AppLayoutInner />
    </AuthGuard>
  );
}
