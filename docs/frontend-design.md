# Reimbee 前端详细设计文档

> 版本: v1.0 | 日期: 2026-07-06 | 状态: 待审核
>
> 基于 Ant Design 5 的现代科技简约风 SPA 前端设计

---

## 目录

1. [技术栈确认](#1-技术栈确认)
2. [项目结构](#2-项目结构)
3. [全局布局设计](#3-全局布局设计)
4. [主题配置——现代科技简约风](#4-主题配置现代科技简约风)
5. [路由设计](#5-路由设计)
6. [状态管理设计](#6-状态管理设计)
7. [API 客户端层](#7-api-客户端层)
8. [SSE 流式对话客户端](#8-sse-流式对话客户端)
9. [页面设计](#9-页面设计)
10. [通用组件设计](#10-通用组件设计)
11. [数据流场景](#11-数据流场景)
12. [错误处理策略](#12-错误处理策略)
13. [构建与部署](#13-构建与部署)

---

## 1. 技术栈确认

| 分类 | 选型 | 版本 | 说明 |
|------|------|------|------|
| 框架 | React | ^18.3 | 函数组件 + Hooks |
| 构建 | Vite | ^6 | SPA 最优解 |
| 语言 | TypeScript | ^5.5 | Strict 模式 |
| UI 库 | Ant Design | ^5 | 企业级组件库 |
| 图标 | @ant-design/icons | ^5 | Ant Design 官方图标 |
| 布局 | @ant-design/pro-layout | ^7 | 开箱即用后台布局 |
| 路由 | react-router-dom | ^7 | 含 lazy loading |
| 状态管理 | zustand | ^5 | 轻量无 boilerplate |
| 图表 | recharts | ^2 | React 声明式图表 |
| HTTP | 原生 fetch | — | SSE 用 EventSource |
| 样式 | TailwindCSS | ^4 | 布局微调 + 自定义样式 |
| 日期 | dayjs | ^1 | Ant Design 内置依赖 |
| 字体 | Inter | — | 现代几何感 sans-serif |
| 测试 | Vitest + RTL | ^3 / ^16 | Vite 生态 |
| 包管理 | pnpm | ^9 | 快，磁盘友好 |

> **不引入**：Redux（Zustand 替代）、axios（fetch 替代）、Less/Sass（TailwindCSS + antd-style 替代）、Emotion/styled-components（Ant Design 内置 CSS-in-JS）

---

## 2. 项目结构

```
reimbee-web/
├── public/
│   └── favicon.svg
├── src/
│   ├── api/                    # API 调用层
│   │   ├── client.ts           # HTTP 客户端（JWT 拦截、错误处理、baseURL）
│   │   ├── sse.ts              # SSE EventSource 客户端封装
│   │   ├── auth.ts             # 登录/注册 API
│   │   ├── departments.ts      # 部门 CRUD
│   │   ├── employees.ts        # 员工 CRUD
│   │   ├── budgets.ts          # 预算查询/看板
│   │   ├── reimbursements.ts   # 报销 CRUD + 提交/审批/驳回
│   │   └── approvals.ts        # 审批操作
│   │
│   ├── stores/                 # Zustand 状态管理
│   │   ├── authStore.ts        # JWT token + 当前用户信息
│   │   ├── chatStore.ts        # 对话消息列表 + SSE 连接状态 + 阶段追踪
│   │   └── appStore.ts         # 全局 UI 状态（侧边栏折叠、暗色模式、通知）
│   │
│   ├── components/             # 通用组件
│   │   ├── layout/             # 布局组件
│   │   │   ├── AppLayout.tsx       # ProLayout 配置 + 菜单 + 用户下拉
│   │   │   └── AuthGuard.tsx       # 路由权限守卫
│   │   ├── chat/               # AI 对话组件
│   │   │   ├── ChatPanel.tsx       # 对话面板（容器）
│   │   │   ├── ChatSidebar.tsx     # 会话列表侧边栏
│   │   │   ├── MessageBubble.tsx   # 消息气泡（支持打字机流式追加）
│   │   │   ├── ThinkingDots.tsx    # AI 思考中动画（三点跳动）
│   │   │   ├── ToolCallCard.tsx    # 工具调用状态卡片（内嵌于消息流）
│   │   │   ├── PhaseIndicator.tsx  # 阶段进度指示器（Steps 组件）
│   │   │   ├── ConfirmModal.tsx    # 确认弹窗（响应 confirm_required 事件）
│   │   │   └── ChatInput.tsx       # 消息输入框 + 附件上传
│   │   └── common/             # 公共展示组件
│   │       ├── AmountText.tsx      # 金额展示（分→元格式化，¥ 前缀）
│   │       ├── StatusTag.tsx       # 报销状态标签（draft/pending/approved/rejected）
│   │       ├── RoleTag.tsx         # 角色标签（employee/approver/admin）
│   │       ├── PageHeader.tsx      # 页面标题栏（含面包屑 + 操作按钮）
│   │       ├── EmptyState.tsx      # 空状态插画
│   │       └── ErrorResult.tsx     # 错误结果页（含重试按钮）
│   │
│   ├── pages/                  # 页面组件
│   │   ├── Login.tsx               # 登录页
│   │   ├── Dashboard.tsx           # 预算看板（首页）
│   │   ├── reimbursement/          # 报销模块
│   │   │   ├── ReimbursementList.tsx    # 报销列表（分页 + 筛选）
│   │   │   ├── ReimbursementDetail.tsx  # 报销详情（含审批时间线）
│   │   │   └── ReimbursementCreate.tsx  # 创建报销单（多票据动态表单）
│   │   ├── approval/
│   │   │   └── PendingApprovals.tsx     # 待审批列表（审批人专属）
│   │   ├── employee/
│   │   │   ├── EmployeeList.tsx         # 员工列表
│   │   │   └── EmployeeForm.tsx         # 创建/编辑员工（Modal 内嵌）
│   │   ├── department/
│   │   │   ├── DepartmentList.tsx       # 部门列表
│   │   │   └── DepartmentForm.tsx       # 创建/编辑部门
│   │   ├── budget/
│   │   │   ├── BudgetManage.tsx         # 预算管理
│   │   │   └── BudgetForm.tsx           # 创建/编辑预算
│   │   └── Chat.tsx                # AI 对话助手页
│   │
│   ├── hooks/                  # 自定义 Hooks
│   │   ├── useSSE.ts               # SSE 连接生命周期管理
│   │   ├── useAuth.ts              # 认证状态检查 + 登出
│   │   └── usePagination.ts        # 分页参数管理
│   │
│   ├── types/                  # TypeScript 类型定义
│   │   ├── api.ts                  # API 请求/响应类型（对应后端 model + dto）
│   │   ├── sse.ts                  # SSE 事件类型枚举（与后端 sse.go 对齐）
│   │   ├── models.ts               # 前端展示模型（含金额转换后的 number）
│   │   └── enums.ts                # 枚举常量（报销状态、角色、费用类别等）
│   │
│   ├── utils/                  # 工具函数
│   │   ├── format.ts               # 金额格式化（分→元 + ¥ 前缀）、日期格式化
│   │   ├── constants.ts            # 前端常量（费用类别映射、角色中文名等）
│   │   └── uuid.ts                 # UUID v7 生成（用于 session_id）
│   │
│   ├── router/
│   │   └── index.tsx               # 路由配置 + 权限守卫 + lazy loading
│   │
│   ├── App.tsx                     # 根组件（ConfigProvider + RouterProvider）
│   ├── main.tsx                    # 入口（渲染根组件）
│   └── vite-env.d.ts
│
├── index.html
├── tailwind.config.ts
├── tsconfig.json
├── vite.config.ts
├── package.json
└── .env.development                # 开发环境变量（API base URL 等）
```

---

## 3. 全局布局设计

### 3.1 布局结构

采用 ProLayout 的 `mix` 布局模式（顶栏 + 侧边栏），对标 Cloudreve 的 AppBar + Drawer 模式：

```
┌──────────────────────────────────────────────────────────┐
│  ProLayout Header (固定顶栏, elevation=0)                 │
│  ┌───────────────────┬────────────────────────┬────────┐ │
│  │  ☰ 折叠按钮       │  面包屑导航              │ 🔔 🌙 👤│ │
│  │  Reimbee Logo     │                        │        │ │
│  └───────────────────┴────────────────────────┴────────┘ │
├────────────┬─────────────────────────────────────────────┤
│            │                                              │
│  Sider     │         Content Area                        │
│  (可折叠)  │         - 带面包屑的页面容器                  │
│            │         - bg: #F5F5F5 (灰色调)               │
│  ┌───────┐ │         - 内容卡片圆角 12px                   │
│  │ 📊 仪表盘│ │         - 自定义滚动条                      │
│  │ 📋 报销  │ │                                            │
│  │ ✅ 审批  │ │         <Outlet />                         │
│  │ 👥 员工  │ │                                            │
│  │ 🏢 部门  │ │                                            │
│  │ 💰 预算  │ │                                            │
│  │ 💬 AI助手│ │                                            │
│  └───────┘ │                                              │
│            │                                              │
└────────────┴─────────────────────────────────────────────┘
```

### 3.2 菜单配置

菜单项根据用户角色动态过滤：

| 菜单项 | 图标 | 路由 | 可见角色 |
|--------|------|------|---------|
| 仪表盘 | `DashboardOutlined` | `/dashboard` | 全部 |
| 报销管理 | `FileTextOutlined` | `/reimbursements` | 全部 |
| 待审批 | `CheckCircleOutlined` | `/reimbursements/pending` | approver, admin |
| 员工管理 | `TeamOutlined` | `/employees` | approver, admin |
| 部门管理 | `BankOutlined` | `/departments` | admin |
| 预算管理 | `DollarOutlined` | `/budgets` | admin |
| AI 助手 | `RobotOutlined` | `/chat` | 全部 |

### 3.3 响应式策略

| 断点 | 宽度 | 布局行为 |
|------|------|---------|
| ≥1200px | 桌面 | 完整双栏：Sider（240px 可折叠） + Content |
| 768-1199px | 平板 | Sider 默认折叠，hover 展开 |
| <768px | 手机 | Sider 变为 Drawer 覆盖模式，Header 简化 |

---

## 4. 主题配置——现代科技简约风

对标 Cloudreve 的 6 项关键设计决策，在 Ant Design 5 中通过 ConfigProvider + TailwindCSS 实现：

```tsx
// 主题配置常量 theme.ts

import type { ThemeConfig } from 'antd';

/** 对标 Cloudreve 的现代科技简约风主题 */
export const reimbeeTheme: ThemeConfig = {
  // ── ① 全局圆角 12px（对标 Cloudreve shape.borderRadius: 12）──
  token: {
    borderRadius: 12,           // 按钮/卡片/输入框/弹窗统一大圆角
    borderRadiusLG: 16,         // 大组件（Modal/Drawer）
    borderRadiusSM: 8,          // 小元素（Tag/Badge）

    // ── ③ 灰白背景（对标 Cloudreve grey[100]）──
    colorBgLayout: '#F5F5F5',   // 页面底色——灰白而非纯白！
    colorBgContainer: '#FFFFFF', // 卡片/表格底色——纯白形成层次

    // 字体
    fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
    fontSize: 14,

    // 颜色微调
    colorPrimary: '#1677FF',    // 保持 Ant Design 默认蓝（科技感）
    colorSuccess: '#52C41A',
    colorWarning: '#FAAD14',
    colorError: '#FF4D4F',

    // 控件高度统一
    controlHeight: 36,
    controlHeightLG: 42,
    controlHeightSM: 30,

    // 内边距
    paddingContentHorizontal: 24,
    paddingContentVertical: 20,
  },

  // ── ② 组件级覆盖 ──
  components: {
    // 按钮：无大写、扁平化（对标 Cloudreve disableElevation + textTransform: "none"）
    Button: {
      fontWeight: 500,
      paddingInline: 20,
      paddingBlock: 6,
    },

    // 菜单：药丸形活跃项（对标 Cloudreve borderRadius: "90px"）
    Menu: {
      itemBorderRadius: 90,       // ④ 超椭圆活跃菜单项
      itemMarginInline: 8,
      itemHeight: 40,
      iconSize: 18,
      collapsedIconSize: 20,
      darkItemBg: 'transparent',
      darkItemSelectedBg: 'rgba(22, 119, 255, 0.15)',
    },

    // 表格：去除分割线，增大行高
    Table: {
      headerBg: '#FAFAFA',
      rowHoverBg: '#F0F5FF',
      borderColor: '#F0F0F0',
      cellPaddingBlock: 12,
      cellPaddingInline: 16,
    },

    // 卡片：微投影，增加层次
    Card: {
      paddingLG: 24,
    },

    // 输入框：填充模式更现代（对标 Cloudreve FilledInput）
    Input: {
      paddingInline: 16,
    },

    // 标签页
    Tabs: {
      horizontalMargin: '0 0 16px 0',
    },

    // 弹窗
    Modal: {
      paddingContentHorizontal: 24,
    },

    // 步骤条：图标更小
    Steps: {
      iconSize: 28,
      dotSize: 8,
    },
  },
};
```

### 4.1 自定义滚动条（对标 Cloudreve ⑤）

通过 TailwindCSS 的 `@layer base` 注入全局滚动条样式：

```css
/* 对标 Cloudreve 的自定义滚动条 —— Chrome/Safari */
::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}
::-webkit-scrollbar-track {
  border-radius: 4px;
  background: transparent;
}
::-webkit-scrollbar-track:hover {
  background: #F0F0F0;
}
::-webkit-scrollbar-thumb {
  border-radius: 4px;
  background: transparent;
  transition: background 0.2s;
}
*:hover::-webkit-scrollbar-thumb {
  background: #D9D9D9;
}
::-webkit-scrollbar-thumb:hover {
  background: #1677FF !important;
}

/* Firefox */
* {
  scrollbar-width: thin;
  scrollbar-color: transparent transparent;
}
```

---

## 5. 路由设计

### 5.1 路由表

```tsx
// router/index.tsx
import { createBrowserRouter, Navigate } from 'react-router-dom';
import { lazy } from 'react';

// ── 布局（非懒加载，始终需要）──
import { AppLayout } from '@/components/layout/AppLayout';

// ── 页面懒加载 ──
const Login = lazy(() => import('@/pages/Login'));
const Dashboard = lazy(() => import('@/pages/Dashboard'));
const ReimbursementList = lazy(() => import('@/pages/reimbursement/ReimbursementList'));
const ReimbursementDetail = lazy(() => import('@/pages/reimbursement/ReimbursementDetail'));
const ReimbursementCreate = lazy(() => import('@/pages/reimbursement/ReimbursementCreate'));
const PendingApprovals = lazy(() => import('@/pages/approval/PendingApprovals'));
const EmployeeList = lazy(() => import('@/pages/employee/EmployeeList'));
const DepartmentList = lazy(() => import('@/pages/department/DepartmentList'));
const BudgetManage = lazy(() => import('@/pages/budget/BudgetManage'));
const Chat = lazy(() => import('@/pages/Chat'));

export const router = createBrowserRouter([
  // ── 无布局路由（登录页）──
  {
    path: '/login',
    element: <Login />,
  },

  // ── AppLayout 包裹的路由（需要认证）──
  {
    path: '/',
    element: <AppLayout />,      // ProLayout + 认证守卫
    errorElement: <ErrorResult />, // 全局错误边界
    children: [
      // 默认重定向
      { index: true, element: <Navigate to="/dashboard" replace /> },

      // 仪表盘（所有角色）
      {
        path: 'dashboard',
        element: <Dashboard />,
      },

      // 报销管理
      {
        path: 'reimbursements',
        element: <ReimbursementList />,
      },
      {
        path: 'reimbursements/create',
        element: <ReimbursementCreate />,
      },
      {
        path: 'reimbursements/:id',
        element: <ReimbursementDetail />,
      },

      // 待审批（approver + admin）
      {
        path: 'reimbursements/pending',
        element: <PendingApprovals />,
      },

      // 员工管理（approver + admin）
      {
        path: 'employees',
        element: <EmployeeList />,
      },

      // 部门管理（仅 admin）
      {
        path: 'departments',
        element: <DepartmentList />,
      },

      // 预算管理（仅 admin）
      {
        path: 'budgets',
        element: <BudgetManage />,
      },

      // AI 对话助手（所有角色）
      {
        path: 'chat',
        element: <Chat />,
      },
      {
        path: 'chat/:sessionId',
        element: <Chat />,
      },

      // 404
      { path: '*', element: <ErrorResult status="404" /> },
    ],
  },
]);
```

### 5.2 权限守卫

```tsx
// components/layout/AuthGuard.tsx

import { useAuthStore } from '@/stores/authStore';
import { Navigate, useLocation } from 'react-router-dom';
import type { ReactNode } from 'react';

/** 角色类型 */
type Role = 'employee' | 'approver' | 'admin';

/** 权限配置：每个路由路径前缀对应的最低角色要求 */
const ROUTE_ROLE_MAP: Record<string, Role> = {
  '/reimbursements/pending': 'approver', // 待审批
  '/employees':              'approver', // 员工管理
  '/departments':            'admin',    // 部门管理（仅管理员可看）
  '/budgets':                'admin',    // 预算管理
};

interface AuthGuardProps {
  children: ReactNode;
}

/** 路由级权限守卫 —— 检查用户角色是否匹配路由要求 */
export function AuthGuard({ children }: AuthGuardProps) {
  const { user, isAuthenticated } = useAuthStore();
  const location = useLocation();

  // 未登录 → 跳转登录页
  if (!isAuthenticated || !user) {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }

  // 查找当前路径对应的角色要求
  const requiredRole = Object.entries(ROUTE_ROLE_MAP).find(
    ([prefix]) => location.pathname.startsWith(prefix)
  );

  if (requiredRole) {
    const [, role] = requiredRole;
    const roles: Role[] = ['employee', 'approver', 'admin'];
    const userRoleIndex = roles.indexOf(user.role as Role);
    const requiredRoleIndex = roles.indexOf(role);

    // 用户角色等级不足 → 403
    if (userRoleIndex < requiredRoleIndex) {
      return <ErrorResult status="403" message="您没有访问此页面的权限" />;
    }
  }

  return <>{children}</>;
}
```

---

## 6. 状态管理设计

### 6.1 authStore —— 认证状态

```ts
// stores/authStore.ts
import { create } from 'zustand';
import { persist } from 'zustand/middleware';

interface User {
  id: number;
  employee_id: string;
  name: string;
  department_id: number;
  email?: string;
  role: 'employee' | 'approver' | 'admin';
  is_approver: boolean;
}

interface AuthState {
  /** JWT token */
  token: string | null;
  /** 当前登录用户信息 */
  user: User | null;
  /** 是否已认证（token 存在且未过期） */
  isAuthenticated: boolean;

  // Actions
  login: (token: string, user: User) => void;
  logout: () => void;
  setUser: (user: User) => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      isAuthenticated: false,

      login: (token, user) => set({
        token,
        user,
        isAuthenticated: true,
      }),

      logout: () => {
        localStorage.clear(); // 清除所有持久化数据
        set({ token: null, user: null, isAuthenticated: false });
      },

      setUser: (user) => set({ user }),
    }),
    {
      name: 'reimbee-auth', // localStorage key
    }
  )
);
```

### 6.2 chatStore —— 对话状态（SSE 驱动）

```ts
// stores/chatStore.ts
import { create } from 'zustand';
import type { SSEEventType } from '@/types/sse';

/** 单条消息的类型 */
type MessageRole = 'user' | 'assistant' | 'system';

interface ChatMessage {
  id: string;
  role: MessageRole;
  content: string;
  timestamp: number;
  /** 是否为流式输出中（仅 assistant 消息） */
  isStreaming?: boolean;
  /** 关联的工具调用 */
  toolCalls?: ToolCallRecord[];
}

/** 工具调用的执行记录 */
interface ToolCallRecord {
  id: string;
  toolName: string;
  status: 'running' | 'success' | 'error';
  input?: unknown;
  output?: unknown;
  errorMessage?: string;
}

/** 报销流程阶段 */
type ReimbPhase = 'idle' | 'phase1_collect' | 'phase2_validate' | 'phase3_execute';

interface ConfirmPrompt {
  action: string;        // confirm_invoice / confirm_submit
  prompt: string;        // 确认提示文字
  context?: unknown;     // 确认所需的上下文数据
}

interface ChatState {
  // ── 会话 ──
  sessionId: string | null;
  messages: ChatMessage[];

  // ── SSE 连接状态 ──
  connectionStatus: 'disconnected' | 'connecting' | 'connected' | 'error';
  reconnectAttempts: number;

  // ── AI 思考状态 ──
  isThinking: boolean;
  thinkingMessage: string;

  // ── 当前流式输出 ──
  currentStreamingMessageId: string | null;

  // ── 报销流程阶段 ──
  currentPhase: ReimbPhase;

  // ── 确认弹窗 ──
  confirmPrompt: ConfirmPrompt | null;

  // ── 会话列表 ──
  sessions: { id: string; title: string; updatedAt: number }[];

  // Actions
  initSession: (sessionId?: string) => string;  // 初始化或恢复 session
  addUserMessage: (content: string) => void;
  startStreamingMessage: () => string;           // 创建空的流式消息，返回 id
  appendStreamContent: (messageId: string, chunk: string) => void;
  finishStreamingMessage: (messageId: string) => void;
  setThinking: (message: string) => void;
  clearThinking: () => void;
  addToolCall: (messageId: string, call: ToolCallRecord) => void;
  updateToolCall: (messageId: string, callId: string, update: Partial<ToolCallRecord>) => void;
  setPhase: (phase: ReimbPhase) => void;
  setConfirmPrompt: (prompt: ConfirmPrompt | null) => void;
  setConnectionStatus: (status: ChatState['connectionStatus']) => void;
  clearMessages: () => void;
  removeSession: (sessionId: string) => void;
}

export const useChatStore = create<ChatState>()((set, get) => ({
  sessionId: null,
  messages: [],
  connectionStatus: 'disconnected',
  reconnectAttempts: 0,
  isThinking: false,
  thinkingMessage: '',
  currentStreamingMessageId: null,
  currentPhase: 'idle',
  confirmPrompt: null,
  sessions: [],

  initSession: (sessionId) => {
    const id = sessionId ?? generateUUIDv7();
    set((state) => ({
      sessionId: id,
      messages: [],
      currentPhase: 'idle',
      // 如果 session 不在列表中则添加
      sessions: state.sessions.some(s => s.id === id)
        ? state.sessions
        : [...state.sessions, { id, title: '新对话', updatedAt: Date.now() }],
    }));
    return id;
  },

  addUserMessage: (content) => set((state) => ({
    messages: [...state.messages, {
      id: generateUUIDv7(),
      role: 'user',
      content,
      timestamp: Date.now(),
    }],
  })),

  startStreamingMessage: () => {
    const id = generateUUIDv7();
    set((state) => ({
      messages: [...state.messages, {
        id,
        role: 'assistant',
        content: '',
        timestamp: Date.now(),
        isStreaming: true,
        toolCalls: [],
      }],
      currentStreamingMessageId: id,
    }));
    return id;
  },

  appendStreamContent: (messageId, chunk) => set((state) => ({
    messages: state.messages.map((m) =>
      m.id === messageId ? { ...m, content: m.content + chunk } : m
    ),
  })),

  finishStreamingMessage: (messageId) => set((state) => ({
    messages: state.messages.map((m) =>
      m.id === messageId ? { ...m, isStreaming: false } : m
    ),
    currentStreamingMessageId: null,
  })),

  setThinking: (message) => set({ isThinking: true, thinkingMessage: message }),
  clearThinking: () => set({ isThinking: false, thinkingMessage: '' }),

  addToolCall: (messageId, call) => set((state) => ({
    messages: state.messages.map((m) =>
      m.id === messageId
        ? { ...m, toolCalls: [...(m.toolCalls ?? []), call] }
        : m
    ),
  })),

  updateToolCall: (messageId, callId, update) => set((state) => ({
    messages: state.messages.map((m) =>
      m.id === messageId
        ? {
            ...m,
            toolCalls: m.toolCalls?.map((tc) =>
              tc.id === callId ? { ...tc, ...update } : tc
            ),
          }
        : m
    ),
  })),

  setPhase: (phase) => set({ currentPhase: phase }),

  setConfirmPrompt: (prompt) => set({ confirmPrompt: prompt }),

  setConnectionStatus: (status) => set({ connectionStatus: status }),

  clearMessages: () => set((state) => {
    // 更新 session 的 updatedAt
    const sid = state.sessionId;
    return {
      messages: [],
      currentPhase: 'idle',
      sessions: sid
        ? state.sessions.map((s) => s.id === sid ? { ...s, updatedAt: Date.now() } : s)
        : state.sessions,
    };
  }),

  removeSession: (sessionId) => set((state) => ({
    sessions: state.sessions.filter((s) => s.id !== sessionId),
  })),
}));

// UUID v7 生成（时间排序 + 随机）
function generateUUIDv7(): string {
  // 简化实现：timestamp (48bit) + random (74bit)
  const ts = Date.now().toString(16).padStart(12, '0');
  const rand = crypto.getRandomValues(new Uint8Array(10));
  const randHex = Array.from(rand, b => b.toString(16).padStart(2, '0')).join('');
  return `${ts.slice(0,8)}-${ts.slice(8)}-7${randHex.slice(0,3)}-${randHex.slice(3,7)}-${randHex.slice(7)}`;
}
```

### 6.3 appStore —— 全局 UI 状态

```ts
// stores/appStore.ts
import { create } from 'zustand';

interface AppState {
  /** 侧边栏是否折叠 */
  sidebarCollapsed: boolean;
  /** 暗色模式 */
  darkMode: boolean;

  toggleSidebar: () => void;
  setSidebarCollapsed: (collapsed: boolean) => void;
  toggleDarkMode: () => void;
}

export const useAppStore = create<AppState>()((set) => ({
  sidebarCollapsed: false,
  darkMode: false,

  toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
  setSidebarCollapsed: (collapsed) => set({ sidebarCollapsed: collapsed }),
  toggleDarkMode: () => set((s) => ({ darkMode: !s.darkMode })),
}));
```

---

## 7. API 客户端层

### 7.1 HTTP 客户端

```ts
// api/client.ts

import { useAuthStore } from '@/stores/authStore';

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080';

/** 通用请求配置 */
interface RequestConfig extends Omit<RequestInit, 'body'> {
  /** 是否为公开接口（无需 JWT） */
  public?: boolean;
  /** 请求体（自动 JSON 序列化） */
  body?: unknown;
  /** 查询参数 */
  params?: Record<string, string | number | undefined>;
}

/** HTTP 客户端——基于 fetch 封装，自动注入 JWT + 统一错误处理 */
async function request<T = unknown>(
  method: string,
  path: string,
  config: RequestConfig = {},
): Promise<T> {
  const { public: isPublic, body, params, ...init } = config;

  // 构建 URL（含查询参数）
  const url = new URL(path, BASE_URL);
  if (params) {
    Object.entries(params).forEach(([k, v]) => {
      if (v !== undefined) url.searchParams.set(k, String(v));
    });
  }

  // 构建请求头
  const headers = new Headers(init.headers);
  headers.set('Content-Type', 'application/json');

  // 非公开接口注入 JWT
  if (!isPublic) {
    const token = useAuthStore.getState().token;
    if (token) {
      headers.set('Authorization', `Bearer ${token}`);
    }
  }

  const response = await fetch(url.toString(), {
    ...init,
    method,
    headers,
    body: body != null ? JSON.stringify(body) : undefined,
  });

  // 统一错误处理
  if (!response.ok) {
    if (response.status === 401) {
      useAuthStore.getState().logout();
      window.location.href = '/login';
      throw new Error('认证已过期，请重新登录');
    }

    const errorBody = await response.json().catch(() => ({}));
    const message = errorBody.error ?? `请求失败 (${response.status})`;
    throw new Error(message);
  }

  // 204 无内容
  if (response.status === 204) return undefined as T;

  return response.json();
}

// ── 便捷方法 ──
export const api = {
  get:    <T>(path: string, config?: RequestConfig) => request<T>('GET', path, config),
  post:   <T>(path: string, body?: unknown, config?: RequestConfig) => request<T>('POST', path, { ...config, body }),
  put:    <T>(path: string, body?: unknown, config?: RequestConfig) => request<T>('PUT', path, { ...config, body }),
  delete: <T>(path: string, config?: RequestConfig) => request<T>('DELETE', path, config),
};

/** 分页响应通用结构 */
export interface PaginatedResponse<T> {
  list: T[];
  total: number;
  page: number;
}
```

### 7.2 业务 API 模块示例

```ts
// api/reimbursements.ts
import { api, type PaginatedResponse } from './client';
import type { Reimbursement, ReimbursementListItem } from '@/types/models';

/** 报销单列表（分页） */
export function listReimbursements(params: {
  page?: number;
  page_size?: number;
  employee_id?: string;
}) {
  return api.get<PaginatedResponse<ReimbursementListItem>>('/api/reimbursements', { params });
}

/** 报销单详情 */
export function getReimbursement(id: number) {
  return api.get<Reimbursement>(`/api/reimbursements/${id}`);
}

/** 按单号查询 */
export function getReimbursementByNo(no: string) {
  return api.get<Reimbursement>(`/api/reimbursements/no/${encodeURIComponent(no)}`);
}

/** 创建报销单 */
export function createReimbursement(data: {
  employee_id: string;
  employee_name: string;
  department_id: number;
  submit_note?: string;
}) {
  return api.post<Reimbursement>('/api/reimbursements', data);
}

/** 提交报销单 */
export function submitReimbursement(id: number, total_amount: number) {
  return api.post<Reimbursement>(`/api/reimbursements/${id}/submit`, { total_amount });
}

/** 审批通过（强制） */
export function approveReimbursement(id: number) {
  return api.post<void>(`/api/reimbursements/${id}/approve`);
}

/** 驳回 */
export function rejectReimbursement(id: number, reason: string) {
  return api.post<void>(`/api/reimbursements/${id}/reject`, { reason });
}

/** 待审批列表 */
export function listPendingReimbursements() {
  return api.get<ReimbursementListItem[]>('/api/reimbursements/pending');
}
```

---

## 8. SSE 流式对话客户端

这是前端最核心的模块——封装 EventSource 连接、8 种事件分发、自动重连、连接状态管理。

### 8.1 类型定义（与后端 sse.go 对齐）

```ts
// types/sse.ts

/** SSE 事件类型枚举——与后端 internal/domain/agent/sse.go 严格对齐 */
export type SSEEventType =
  | 'thinking'
  | 'tool_call'
  | 'tool_result'
  | 'message'
  | 'phase_change'
  | 'confirm_required'
  | 'error'
  | 'done';

/** 通用 SSE 事件结构 */
export interface SSEEvent {
  type: SSEEventType;
  data: unknown;
}

export interface ThinkingData {
  message: string;
}

export interface ToolCallData {
  tool: string;
  input: unknown;
}

export interface ToolResultData {
  tool: string;
  output: unknown;
}

export interface MessageData {
  content: string;
  /** true=流式增量（打字机追加），false=完整内容（替换） */
  delta: boolean;
}

export interface PhaseChangeData {
  from: string;
  to: string;
  summary: string;
}

export interface ConfirmRequiredData {
  prompt: string;
  action: string;
  context: unknown;
}

export interface ErrorData {
  message: string;
  /** 是否可重试 */
  retry: boolean;
  /** 错误代码 */
  code: string;
}
```

### 8.2 SSE 客户端 Hook

```ts
// hooks/useSSE.ts

import { useEffect, useRef, useCallback } from 'react';
import { useChatStore } from '@/stores/chatStore';
import { useAuthStore } from '@/stores/authStore';
import type {
  SSEEvent, ThinkingData, ToolCallData, ToolResultData,
  MessageData, PhaseChangeData, ConfirmRequiredData, ErrorData,
} from '@/types/sse';

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080';

/** 最大重连次数 */
const MAX_RECONNECT = 5;
/** 重连基础延迟（ms），指数退避：1s → 2s → 4s → 8s → ... */
const RECONNECT_BASE_DELAY = 1000;

/**
 * SSE 连接管理 Hook
 *
 * 职责：
 * 1. 创建 EventSource 连接到 /api/chat/stream
 * 2. 监听 8 种 SSE 事件类型，分发到 chatStore
 * 3. 连接断开时自动重连（指数退避，最多 5 次）
 * 4. 组件卸载时关闭连接
 *
 * @param sessionId - 会话 ID（UUID v7）
 * @param message - 待发送的用户消息（null 时不发送，仅监听）
 */
export function useSSE(sessionId: string | null, message: string | null) {
  const eventSourceRef = useRef<EventSource | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout>>();
  const reconnectCountRef = useRef(0);

  const store = useChatStore();
  const token = useAuthStore((s) => s.token);

  // 事件处理器映射
  const handleEvent = useCallback((eventType: string, rawData: string) => {
    if (!rawData) return;
    const parsed: SSEEvent = JSON.parse(rawData);

    switch (parsed.type as SSEEvent['type']) {
      // ── thinking: AI 思考中 ──
      case 'thinking': {
        const data = parsed.data as ThinkingData;
        store.setThinking(data.message);
        break;
      }

      // ── tool_call: 工具调用开始 ──
      case 'tool_call': {
        const data = parsed.data as ToolCallData;
        const streamingId = store.currentStreamingMessageId;
        if (streamingId) {
          store.addToolCall(streamingId, {
            id: `${data.tool}-${Date.now()}`,
            toolName: data.tool,
            status: 'running',
            input: data.input,
          });
        }
        break;
      }

      // ── tool_result: 工具调用完成 ──
      case 'tool_result': {
        const data = parsed.data as ToolResultData;
        const streamingId = store.currentStreamingMessageId;
        if (streamingId) {
          // 找到最近一个 matching toolCall（可能有多个）
          const msgs = useChatStore.getState().messages;
          const msg = msgs.find((m) => m.id === streamingId);
          const lastCall = msg?.toolCalls
            ?.filter((tc) => tc.toolName === data.tool && tc.status === 'running')
            .pop();
          if (lastCall) {
            store.updateToolCall(streamingId, lastCall.id, {
              status: 'success',
              output: data.output,
            });
          }
        }
        break;
      }

      // ── message: LLM 文本输出 ──
      case 'message': {
        const data = parsed.data as MessageData;

        if (data.delta) {
          // 流式增量——获取或创建流式消息
          let msgId = useChatStore.getState().currentStreamingMessageId;
          if (!msgId) {
            msgId = store.startStreamingMessage();
          }
          store.appendStreamContent(msgId, data.content);
        } else {
          // 完整消息——直接追加
          store.finishStreamingMessage(store.startStreamingMessage());
          const msgId = useChatStore.getState().currentStreamingMessageId!;
          store.appendStreamContent(msgId, data.content);
          store.finishStreamingMessage(msgId);
        }
        break;
      }

      // ── phase_change: 阶段切换 ──
      case 'phase_change': {
        const data = parsed.data as PhaseChangeData;
        store.setPhase(data.to as 'idle' | 'phase1_collect' | 'phase2_validate' | 'phase3_execute');
        break;
      }

      // ── confirm_required: 需要用户确认 ──
      case 'confirm_required': {
        const data = parsed.data as ConfirmRequiredData;
        store.setConfirmPrompt({
          action: data.action,
          prompt: data.prompt,
          context: data.context,
        });
        break;
      }

      // ── error: 错误事件 ──
      case 'error': {
        const data = parsed.data as ErrorData;
        console.error(`[SSE Error] ${data.code}: ${data.message}`);

        // 终止当前流式消息
        const msgId = useChatStore.getState().currentStreamingMessageId;
        if (msgId) {
          store.appendStreamContent(msgId, `\n\n❌ ${data.message}`);
          store.finishStreamingMessage(msgId);
        }
        store.clearThinking();

        // 如果不可重试，关闭连接
        if (!data.retry) {
          eventSourceRef.current?.close();
          store.setConnectionStatus('error');
        }
        break;
      }

      // ── done: 对话结束 ──
      case 'done': {
        const msgId = useChatStore.getState().currentStreamingMessageId;
        if (msgId) {
          store.finishStreamingMessage(msgId);
        }
        store.clearThinking();
        store.setConnectionStatus('disconnected');
        eventSourceRef.current?.close();
        reconnectCountRef.current = 0; // 重置重连计数
        break;
      }

      default:
        break;
    }
  }, [store]);

  // 建立 SSE 连接
  useEffect(() => {
    if (!sessionId || !message || !token) return;

    // 重置重连计数
    reconnectCountRef.current = 0;

    const connect = () => {
      // 关闭旧连接
      eventSourceRef.current?.close();

      const url = new URL('/api/chat/stream', BASE_URL);
      url.searchParams.set('session_id', sessionId);
      url.searchParams.set('message', message);

      store.setConnectionStatus('connecting');

      // ⚠️ EventSource 不支持自定义 Header！
      // 方案：使用 URL query 参数传递 token，后端需支持 `?token=xxx` 或通过 Cookie
      // 临时方案：将 token 加入 URL（后端需适配，生产环境改用 Cookie）
      url.searchParams.set('token', token);

      const es = new EventSource(url.toString());
      eventSourceRef.current = es;

      es.onopen = () => {
        store.setConnectionStatus('connected');
        reconnectCountRef.current = 0;
      };

      // ── 注册 8 种事件类型监听 ──
      const eventTypes: SSEEvent['type'][] = [
        'thinking', 'tool_call', 'tool_result', 'message',
        'phase_change', 'confirm_required', 'error', 'done',
      ];

      eventTypes.forEach((type) => {
        es.addEventListener(type, (e: MessageEvent) => {
          handleEvent(type, e.data);
        });
      });

      // 通用 onerror（连接中断）
      es.onerror = () => {
        store.setConnectionStatus('error');

        if (reconnectCountRef.current < MAX_RECONNECT) {
          const delay = RECONNECT_BASE_DELAY * Math.pow(2, reconnectCountRef.current);
          reconnectCountRef.current++;

          reconnectTimerRef.current = setTimeout(() => {
            connect();
          }, delay);
        } else {
          store.setConnectionStatus('error');
          store.clearThinking();
        }
      };
    };

    connect();

    // 清理函数
    return () => {
      eventSourceRef.current?.close();
      clearTimeout(reconnectTimerRef.current);
    };
  }, [sessionId, message, token, handleEvent]);
}
```

### 8.3 SSE 客户端架构图

```
用户输入 "我要报销"
         │
         ▼
┌─ ChatInput ─────────────────────────────────────┐
│  1. 生成 sessionId (UUID v7)                    │
│  2. 写入 chatStore.initSession()                │
│  3. 写入 chatStore.addUserMessage()             │
│  4. 触发 useSSE(sessionId, message)             │
└─────────────────────────────────────────────────┘
         │
         ▼
┌─ useSSE Hook ───────────────────────────────────┐
│                                                 │
│  new EventSource(                               │
│    /api/chat/stream                             │
│    ?session_id=xxx&message=xxx&token=xxx        │
│  )                                              │
│                                                 │
│  监听 8 种事件:                                  │
│  ┌──────────────┬─────────────────────────────┐ │
│  │ thinking     │ → chatStore.setThinking()    │ │
│  │ tool_call    │ → chatStore.addToolCall()    │ │
│  │ tool_result  │ → chatStore.updateToolCall() │ │
│  │ message      │ → delta? appendContent()     │ │
│  │              │   : finishStreamingMessage() │ │
│  │ phase_change │ → chatStore.setPhase()       │ │
│  │ confirm      │ → chatStore.setConfirmPrompt │ │
│  │ error        │ → 若不可重试则 close()       │ │
│  │ done         │ → finishStreamingMessage()   │ │
│  │              │   + close() + 重置重连计数    │ │
│  └──────────────┴─────────────────────────────┘ │
│                                                 │
│  自动重连: 指数退避 1s→2s→4s→8s, 最多 5 次     │
└─────────────────────────────────────────────────┘
         │
         ▼
┌─ ChatPanel UI 响应式渲染 ───────────────────────┐
│                                                 │
│  isThinking    → <ThinkingDots /> 三点跳动      │
│  phase         → <PhaseIndicator /> 步骤条      │
│  messages[]    → <MessageBubble /> 消息列表      │
│    .isStreaming → 打字机追加 + 光标闪烁         │
│    .toolCalls[] → <ToolCallCard /> 内嵌卡片    │
│  confirmPrompt → <ConfirmModal /> 确认弹窗      │
│                                                 │
└─────────────────────────────────────────────────┘
```

---

## 9. 页面设计

### 9.1 登录页 `/login`

| 属性 | 说明 |
|------|------|
| **布局** | 居中卡片，全屏渐变背景（蓝→紫或深浅灰色调） |
| **组件** | Ant Design Form + Input + Button + message |
| **状态** | 默认 / loading（按钮禁用+spin） / error（message.error） |
| **交互** | 提交→调用 `POST /api/auth/login`→成功存入 authStore→跳转 `/dashboard` |

```
┌──────────────────────────────────────────────────┐
│                                                  │
│                                                  │
│          ┌──────────────────────────┐            │
│          │    🏦  Reimbee           │            │
│          │    企业财务报销助手        │            │
│          │                          │            │
│          │  ┌──────────────────────┐│            │
│          │  │  工号                ││            │
│          │  └──────────────────────┘│            │
│          │  ┌──────────────────────┐│            │
│          │  │  密码                ││            │
│          │  └──────────────────────┘│            │
│          │  ┌──────────────────────┐│            │
│          │  │      登  录          ││            │
│          │  └──────────────────────┘│            │
│          └──────────────────────────┘            │
│                                                  │
└──────────────────────────────────────────────────┘
```

### 9.2 仪表盘 `/dashboard`

| 属性 | 说明 |
|------|------|
| **组件** | Card + Statistic + Progress + Recharts BarChart/PieChart |
| **数据源** | `GET /api/budgets/dashboard?year=2026` |
| **状态** | loading（Skeleton） / data / empty / error |

**卡片布局**：

```
┌──────────────────────────────────────────────────────────┐
│  📊 预算看板 — 2026 财年          [ 年份选择器 ▼ ]        │
├────────────┬────────────┬────────────┬───────────────────┤
│ 总预算      │ 已支出      │ 冻结金额    │ 剩余可用           │
│ ¥500,000  │ ¥150,000  │ ¥50,000   │ ¥300,000         │
│            │ 使用率 30%  │            │                   │
├────────────┴────────────┴────────────┴───────────────────┤
│                                                          │
│  ┌─────────────────────────────────────────────────┐    │
│  │  各部门预算使用情况（横向柱状图）                    │    │
│  │  技术部 ████████████░░░░░░░░░░ 40%              │    │
│  │  市场部 ██████░░░░░░░░░░░░░░░░ 20%              │    │
│  │  财务部 ██░░░░░░░░░░░░░░░░░░░░  5%              │    │
│  └─────────────────────────────────────────────────┘    │
│                                                          │
│  ┌──────────────────────┐  ┌──────────────────────┐     │
│  │ 费用类别分布（饼图）    │  │ 报销状态统计（环形图）   │     │
│  │                      │  │                      │     │
│  └──────────────────────┘  └──────────────────────┘     │
└──────────────────────────────────────────────────────────┘
```

### 9.3 报销列表 `/reimbursements`

| 属性 | 说明 |
|------|------|
| **组件** | Table（分页 + 排序 + 筛选） + Button + Tag + Space |
| **数据源** | `GET /api/reimbursements?page=&page_size=&employee_id=` |
| **状态** | loading（Table skeleton） / data / empty / error |

**表格列定义**：

| 列 | 字段 | 宽度 | 渲染 |
|----|------|------|------|
| 报销单号 | `reimbursement_no` | 180px | 可点击链接→详情页 |
| 申请人 | `employee_name` | 100px | 文本 |
| 部门 | `department` | 100px | 文本 |
| 金额 | `total_amount` | 120px | `<AmountText>`（分→元 + ¥前缀） |
| 状态 | `status` | 100px | `<StatusTag>` 彩色标签 |
| 票据数 | `invoices.length` | 80px | 数字 |
| 提交时间 | `created_at` | 160px | dayjs 格式化 |
| 操作 | — | 120px | "查看详情"按钮 |

**操作栏**：

```
┌──────────────────────────────────────────────────────────┐
│  📋 报销管理                               [+ 新建报销]   │
│  ┌──────────────────────────────────────────────────────┐│
│  │ [工号筛选 ▼] [状态筛选 ▼] [日期范围 📅]  [🔍 搜索]   ││
│  └──────────────────────────────────────────────────────┘│
│  ┌──────────────────────────────────────────────────────┐│
│  │  单号         │ 申请人 │ 部门   │ 金额    │ 状态 │ 操作││
│  │  REIMB-...    │ 张三   │ 技术部 │ ¥2,380 │ 待审批│ →  ││
│  │  REIMB-...    │ 李四   │ 市场部 │ ¥1,500 │ 已通过│ →  ││
│  │  ...                                                ││
│  └──────────────────────────────────────────────────────┘│
│                                    [< 1 2 3 ... 10 >]    │
└──────────────────────────────────────────────────────────┘
```

### 9.4 报销详情 `/reimbursements/:id`

| 属性 | 说明 |
|------|------|
| **组件** | Descriptions + Table（票据明细） + Timeline（审批进度） + Button（操作） |
| **数据源** | `GET /api/reimbursements/:id` + `GET /api/reimbursements/:id/approvals` |
| **状态** | loading / data / error / 404 |

**页面结构**：

```
┌──────────────────────────────────────────────────────────┐
│  ← 返回    报销单详情 — REIMB-2026-0042                   │
├──────────────────────────────────────────────────────────┤
│  ┌─ 基本信息 ──────────────────────────────────────────┐ │
│  │  申请人：张三（EMP001）    部门：技术部              │ │
│  │  金额：¥2,380.00          状态：待审批 🟡            │ │
│  │  事由：深圳出差往返机票 + 住宿                        │ │
│  │  提交时间：2026-07-06 10:00                          │ │
│  └────────────────────────────────────────────────────┘ │
│                                                          │
│  ┌─ 票据明细 ──────────────────────────────────────────┐ │
│  │  # │ 金额     │ 类别       │ 日期       │ 校验结果   │ │
│  │  1 │ ¥1,580  │ 差旅-交通   │ 2026-07-01│ ✅ pass    │ │
│  │  2 │ ¥800    │ 差旅-住宿   │ 2026-07-01│ ✅ pass    │ │
│  └────────────────────────────────────────────────────┘ │
│                                                          │
│  ┌─ 审批进度 ──────────────────────────────────────────┐ │
│  │  ● 李主管（待审批）                                  │ │
│  │  ○ 张财务（待审批）                                  │ │
│  └────────────────────────────────────────────────────┘ │
│                                                          │
│  ┌─ 操作 ──────────────────────────────────────────────┐ │
│  │     [✅ 审批通过]   [❌ 驳回]                        │ │
│  └────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────┘
```

### 9.5 创建报销单 `/reimbursements/create`

| 属性 | 说明 |
|------|------|
| **组件** | Form（嵌套动态表单） + Steps（当前步骤高亮） + Upload + InputNumber + Select + Button |
| **数据源** | `POST /api/reimbursements` 创建草稿 → `POST /api/reimbursements/:id/submit` 提交 |
| **状态** | 编辑中 / 提交 loading / 成功跳转 / 校验失败提示 |

**三步骤流程**：

```
┌──────────────────────────────────────────────────────┐
│  Step 1: 基本信息    Step 2: 票据信息    Step 3: 确认提交 │
│  ────────────────    ────────────────    ──────────────── │
│                                                          │
│  ┌─ Step 1：基本信息 ───────────────────────────────┐   │
│  │  申请人：张三（EMP001）  [自动填充，只读]         │   │
│  │  部门：技术部           [自动填充，只读]          │   │
│  │  事由：[______________________]                   │   │
│  │                                                  │   │
│  │                           [下一步 →]              │   │
│  └──────────────────────────────────────────────────┘   │
│                                                          │
│  ┌─ Step 2：票据信息（可添加多张）───────────────────┐   │
│  │  ┌────┬────────┬────────┬──────┬────────┬──────┐ │   │
│  │  │ #  │ 票据图片│ 金额   │ 类别  │ 日期   │ 操作  │ │   │
│  │  │ 1  │ [📎]   │ 1,580 │ 交通  │ 07-01  │ 🗑   │ │   │
│  │  │ 2  │ [📎]   │ 800   │ 住宿  │ 07-01  │ 🗑   │ │   │
│  │  └────┴────────┴────────┴──────┴────────┴──────┘ │   │
│  │  [+ 添加票据]                                     │   │
│  │                                                    │   │
│  │  [← 上一步]              [下一步 →]                │   │
│  └────────────────────────────────────────────────────┘   │
│                                                          │
│  ┌─ Step 3：确认提交 ───────────────────────────────┐   │
│  │  申请人：张三    部门：技术部                      │   │
│  │  票据数：2 张    总金额：¥2,380.00                 │   │
│  │  事由：深圳出差                                   │   │
│  │                                                    │   │
│  │  [← 上一步]              [✅ 确认提交]             │   │
│  └────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────┘
```

### 9.6 AI 对话助手 `/chat`

| 属性 | 说明 |
|------|------|
| **布局** | 左侧会话列表 + 右侧对话区域（对标 ChatGPT/Cloudreve 布局） |
| **组件** | ChatPanel + ChatSidebar + ChatInput + MessageBubble + PhaseIndicator + ConfirmModal |
| **数据源** | SSE 流式端点 `GET /api/chat/stream` |
| **状态** | 无会话 / 思考中 / 流式输出中 / 工具调用中 / 等确认 / 错误 / 完成 |

**布局结构**：

```
┌────────────┬──────────────────────────────────────────────┐
│ 会话列表    │  对话区域                                     │
│            │                                              │
│ [+ 新对话] │  ┌─ PhaseIndicator ──────────────────────┐  │
│            │  │  信息收集 ●─── 校验确认 ○─── 执行提交 ○ │  │
│ ───────── │  └────────────────────────────────────────┘  │
│ 报销差旅费 │                                              │
│ 查询进度   │  ┌───────────────────────────────────────┐  │
│ 预算查询   │  │                                       │  │
│            │  │   👤 用户：我要报销差旅费 500 元       │  │
│            │  │                                       │  │
│            │  │   🤖 AI：好的，请上传您的票据图片...   │  │
│            │  │                                       │  │
│            │  │   ┌─ 🔧 正在识别票据 ──────────────┐ │  │
│            │  │   │  recognize_invoice            │ │  │
│            │  │   │  识别到：差旅-交通 ¥500.00     │ │  │
│            │  │   └──────────────────────────────┘ │  │
│            │  │                                       │  │
│            │  │   🤖 AI：识别完成，请确认票据信息。    │  │
│            │  │                                       │  │
│            │  │   ⚠️ 确认弹窗：请确认票据金额和类别   │  │
│            │  │       [取消]      [✅ 确认]           │  │
│            │  │                                       │  │
│            │  └───────────────────────────────────────┘  │
│            │  ┌───────────────────────────────────────┐  │
│            │  │  [+ 📎 上传票据]  [输入消息...]  [发送] │  │
│            │  └───────────────────────────────────────┘  │
└────────────┴──────────────────────────────────────────────┘
```

### 9.7 待审批列表 `/reimbursements/pending`

| 属性 | 说明 |
|------|------|
| **组件** | Table（无分页，全量展示） + Button（审批/驳回） + Modal（驳回原因） |
| **数据源** | `GET /api/reimbursements/pending` |
| **交互** | 审批→确认弹窗→`POST /approve`；驳回→Modal 输入原因→`POST /reject` |

### 9.8 员工管理 `/employees`

| 属性 | 说明 |
|------|------|
| **组件** | Table（分页） + Button（新增） + Modal（创建/编辑表单） |
| **数据源** | `GET /api/employees` + `POST/PUT/DELETE` |
| **交互** | 新增→Modal 弹出表单（工号自动生成、姓名、部门、邮箱、角色、是否审批人） |

### 9.9 部门管理 `/departments`

| 属性 | 说明 |
|------|------|
| **组件** | Table（分页） + Button（新增） + Modal（创建/编辑表单） |
| **数据源** | `GET /api/departments` + `POST/PUT/DELETE` |
| **交互** | 新增→Modal 弹出表单（名称、主管选择）；删除→二次确认（有关联员工时提示） |

### 9.10 预算管理 `/budgets`

| 属性 | 说明 |
|------|------|
| **组件** | Table（分页） + Button（新增） + Modal（创建/编辑表单） |
| **数据源** | `GET /api/budgets/dashboard`（列表视图）+ `POST/PUT` |
| **交互** | 新增→Modal 弹出表单（部门、财年、金额）；编辑→金额修改 |

---

## 10. 通用组件设计

### 10.1 AmountText —— 金额展示

```tsx
// components/common/AmountText.tsx

interface AmountTextProps {
  /** 金额（分），来自 API */
  amount: number;
  /** 是否显示货币符号，默认 true */
  showSymbol?: boolean;
  /** 小数位数，默认 2 */
  precision?: number;
}

/**
 * 金额展示组件——自动分→元转换 + ¥ 前缀
 * 后端存储 int64（分），前端展示 float64（元）
 *
 * 示例：
 *   <AmountText amount={238000} />  →  ¥2,380.00
 *   <AmountText amount={50000} precision={0} />  →  ¥500
 */
export function AmountText({ amount, showSymbol = true, precision = 2 }: AmountTextProps) {
  const yuan = amount / 100;
  const formatted = yuan.toLocaleString('zh-CN', {
    minimumFractionDigits: precision,
    maximumFractionDigits: precision,
  });
  return <span>{showSymbol ? `¥${formatted}` : formatted}</span>;
}
```

### 10.2 StatusTag —— 报销状态标签

```tsx
// components/common/StatusTag.tsx
import { Tag } from 'antd';

const STATUS_MAP: Record<string, { color: string; label: string }> = {
  draft:     { color: 'default', label: '草稿' },
  pending:   { color: 'processing', label: '待审批' },
  reviewing: { color: 'warning', label: '审批中' },
  approved:  { color: 'success', label: '已通过' },
  rejected:  { color: 'error', label: '已驳回' },
};

export function StatusTag({ status }: { status: string }) {
  const config = STATUS_MAP[status] ?? { color: 'default', label: status };
  return <Tag color={config.color}>{config.label}</Tag>;
}
```

### 10.3 PhaseIndicator —— 报销阶段指示器

```tsx
// components/chat/PhaseIndicator.tsx
import { Steps } from 'antd';
import { useChatStore } from '@/stores/chatStore';

const PHASES = [
  { key: 'phase1_collect', title: '信息收集' },
  { key: 'phase2_validate', title: '校验确认' },
  { key: 'phase3_execute', title: '执行提交' },
];

export function PhaseIndicator() {
  const phase = useChatStore((s) => s.currentPhase);

  if (phase === 'idle') return null;

  const currentIndex = PHASES.findIndex((p) => p.key === phase);

  return (
    <Steps
      size="small"
      current={currentIndex >= 0 ? currentIndex : 0}
      items={PHASES.map((p) => ({
        title: p.title,
      }))}
      style={{ marginBottom: 16 }}
    />
  );
}
```

### 10.4 ThinkingDots —— AI 思考动画

```tsx
// components/chat/ThinkingDots.tsx
import { Typography } from 'antd';
import { useChatStore } from '@/stores/chatStore';

/**
 * AI 思考中动画——三点跳动 + 文字提示
 * 对应 SSE thinking 事件
 */
export function ThinkingDots() {
  const { isThinking, thinkingMessage } = useChatStore();

  if (!isThinking) return null;

  return (
    <div className="flex items-center gap-2 px-4 py-2">
      <span className="thinking-dots">
        <span className="dot" />
        <span className="dot" />
        <span className="dot" />
      </span>
      <Typography.Text type="secondary">{thinkingMessage}</Typography.Text>
    </div>
  );
}
```

```css
/* ThinkingDots.css */
.thinking-dots {
  display: inline-flex;
  gap: 4px;
  align-items: center;
}
.thinking-dots .dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #1677FF;
  animation: thinking-bounce 1.4s infinite ease-in-out both;
}
.thinking-dots .dot:nth-child(1) { animation-delay: -0.32s; }
.thinking-dots .dot:nth-child(2) { animation-delay: -0.16s; }
.thinking-dots .dot:nth-child(3) { animation-delay: 0; }

@keyframes thinking-bounce {
  0%, 80%, 100% { transform: scale(0.5); opacity: 0.4; }
  40% { transform: scale(1); opacity: 1; }
}
```

### 10.5 ConfirmModal —— 确认弹窗

```tsx
// components/chat/ConfirmModal.tsx
import { Modal, Button, Space } from 'antd';
import { useChatStore } from '@/stores/chatStore';

/**
 * 确认弹窗——响应 SSE confirm_required 事件
 * 用户选择后需通过后续对话消息告知 AI（后端无专门的 confirm API）
 */
export function ConfirmModal() {
  const { confirmPrompt, setConfirmPrompt, addUserMessage } = useChatStore();

  if (!confirmPrompt) return null;

  const handleConfirm = () => {
    addUserMessage(`确认操作：${confirmPrompt.action}`);
    setConfirmPrompt(null);
    // TODO: 触发新的 SSE 请求，携带确认结果
  };

  const handleCancel = () => {
    addUserMessage('取消操作');
    setConfirmPrompt(null);
  };

  return (
    <Modal
      title="确认操作"
      open={!!confirmPrompt}
      onCancel={handleCancel}
      footer={
        <Space>
          <Button onClick={handleCancel}>取消</Button>
          <Button type="primary" onClick={handleConfirm}>确认</Button>
        </Space>
      }
    >
      <p>{confirmPrompt.prompt}</p>
    </Modal>
  );
}
```

### 10.6 ToolCallCard —— 工具调用卡片

```tsx
// components/chat/ToolCallCard.tsx
import { Card, Tag, Spin, Collapse } from 'antd';
import {
  ToolOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  LoadingOutlined,
} from '@ant-design/icons';
import type { ToolCallRecord } from '@/stores/chatStore';

const TOOL_LABELS: Record<string, string> = {
  recognize_invoice: '票据识别',
  check_compliance: '合规检查',
  check_budget: '预算检查',
  generate_pdf: '生成 PDF',
  send_email: '发送邮件',
  query_progress: '查询进度',
  query_records: '查询记录',
};

export function ToolCallCard({ call }: { call: ToolCallRecord }) {
  const label = TOOL_LABELS[call.toolName] ?? call.toolName;

  const statusIcon = {
    running: <LoadingOutlined spin style={{ color: '#1677FF' }} />,
    success: <CheckCircleOutlined style={{ color: '#52C41A' }} />,
    error: <CloseCircleOutlined style={{ color: '#FF4D4F' }} />,
  }[call.status];

  return (
    <Card
      size="small"
      className="tool-call-card bg-gray-50"
      styles={{ body: { padding: '8px 12px' } }}
    >
      <div className="flex items-center gap-2">
        <ToolOutlined />
        <span className="font-medium">{label}</span>
        {statusIcon}
        <Tag>{call.status === 'running' ? '执行中' : call.status === 'success' ? '完成' : '失败'}</Tag>
      </div>

      {call.status === 'success' && call.output && (
        <Collapse
          ghost
          size="small"
          items={[{
            key: 'output',
            label: '查看结果',
            children: <pre className="text-xs">{JSON.stringify(call.output, null, 2)}</pre>,
          }]}
        />
      )}

      {call.status === 'error' && call.errorMessage && (
        <p className="text-red-500 text-xs mt-1">{call.errorMessage}</p>
      )}
    </Card>
  );
}
```

---

## 11. 数据流场景

### 11.1 用户登录

```
用户输入工号+密码 → Login.tsx
  → POST /api/auth/login { employee_id, password }
  → 200: { token, employee: { id, employee_id, name, role, ... } }
  → authStore.login(token, employee)
  → navigate('/dashboard')
  → 401: message.error('工号或密码错误')
```

### 11.2 创建并提交报销单

```
Step 1: 填写基本信息 → ReimbursementCreate.tsx
  → 收集 employee_id, department_id, submit_note
  → 进入 Step 2

Step 2: 填写票据信息
  → 逐条添加票据（上传图片? → 金额 → 类别 → 日期）
  → 累计 total_amount
  → 进入 Step 3

Step 3: 确认提交
  → POST /api/reimbursements { employee_id, department_id, submit_note }
  → 201: { id, reimbursement_no, status: "draft", ... }
  → POST /api/reimbursements/:id/submit { total_amount }
  → 200 → message.success('提交成功')
  → navigate('/reimbursements')
```

### 11.3 AI 报销对话（完整流程）

```
1. 用户进入 /chat → Chat.tsx 初始化
   → chatStore.initSession() → 生成 UUID v7

2. 用户输入 "我要报销" → ChatInput.tsx
   → chatStore.addUserMessage("我要报销")
   → 触发 useSSE(sessionId, "我要报销")

3. SSE 事件流开始:
   thinking → ThinkingDots "正在理解您的需求..."
   message(delta) → AI: "好的，请上传您的票据图片..."
   → 用户上传票据图片（文件上传组件）

4. Phase 切换到 "phase1_collect":
   phase_change → PhaseIndicator 高亮 Step 1

5. AI 调用 OCR 工具:
   tool_call("recognize_invoice") → ToolCallCard 显示 ⏳
   tool_result({ amount: 50000, category: "差旅-交通" }) → ToolCallCard ✅

6. AI 需要确认:
   confirm_required({ action: "confirm_invoice", prompt: "请确认票据信息" })
   → ConfirmModal 弹出

7. 用户确认:
   → chatStore.addUserMessage("确认")
   → 触发 useSSE(sessionId, "确认")

8. 继续 Phase2 → Phase3 同理...

9. 对话完成:
   done → finishStreamingMessage → EventSource.close()
```

### 11.4 审批驳回流程

```
审批人进入 /reimbursements/pending → PendingApprovals.tsx
  → GET /api/reimbursements/pending → 列表展示

点击 "驳回" → Modal 弹出 → 输入驳回原因
  → POST /api/reimbursements/:id/reject { reason }
  → 200 → message.success('已驳回')
  → 列表自动刷新（移除已驳回项）
```

---

## 12. 错误处理策略

### 12.1 分层错误处理

| 层级 | 策略 | 示例 |
|------|------|------|
| **HTTP 客户端** | `api/client.ts` 拦截 401→登出，其余→throw Error(message) | JWT 过期自动跳转登录页 |
| **页面级** | ErrorBoundary 包裹每个页面，捕获未处理异常 | React 渲染崩溃→ErrorResult 组件 |
| **操作级** | try/catch + antd message.error() | 创建报销失败→红色提示条 |
| **SSE 级** | error 事件监听 + 自动重连（指数退避） | 网络断开→1s/2s/4s/8s 重连 |
| **表单级** | Ant Design Form.validateFields() 捕获校验错误 | 必填为空→字段标红+提示文字 |

### 12.2 错误码映射

```ts
// utils/errors.ts

/** 后端错误码 → 前端用户友好提示 */
const ERROR_MESSAGES: Record<string, string> = {
  'ocr_error':           '票据识别失败，请检查图片是否清晰后重试',
  'budget_insufficient': '部门预算不足，请联系管理员',
  'no_approver':         '未配置审批人，请联系管理员',
  'status_not_allowed':  '当前状态不允许此操作',
  'department_has_employees': '该部门下有关联员工，无法删除',
  'budget_duplicate':    '该部门同一年度已有预算',
  'employee_duplicate':  '该工号已存在',
  'stream_error':        '对话连接中断，正在尝试重连...',
};
```

### 12.3 全局错误边界

```tsx
// components/common/ErrorResult.tsx
import { Result, Button } from 'antd';
import { useRouteError } from 'react-router-dom';

interface ErrorResultProps {
  status?: '403' | '404' | '500';
  message?: string;
}

export function ErrorResult({ status, message }: ErrorResultProps) {
  const routeError = useRouteError();

  const config = {
    '403': { title: '无权限访问', subTitle: message ?? '您没有访问此页面的权限' },
    '404': { title: '页面不存在', subTitle: message ?? '请检查 URL 是否正确' },
    '500': { title: '服务器错误', subTitle: message ?? '请稍后重试' },
  }[status ?? '500'];

  return (
    <Result
      status={status as any}
      title={config.title}
      subTitle={config.subTitle}
      extra={
        <Button type="primary" onClick={() => window.location.reload()}>
          刷新页面
        </Button>
      }
    />
  );
}
```

---

## 13. 构建与部署

### 13.1 Vite 配置要点

```ts
// vite.config.ts
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react-swc';
import tailwindcss from '@tailwindcss/vite';
import { resolve } from 'path';

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
    },
  },
  server: {
    host: '0.0.0.0',
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/uploads': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false, // 生产环境关闭
    rollupOptions: {
      output: {
        manualChunks: {
          'react-vendor': ['react', 'react-dom', 'react-router-dom'],
          'antd-vendor': ['antd', '@ant-design/icons', '@ant-design/pro-layout'],
          'chart-vendor': ['recharts'],
        },
      },
    },
  },
});
```

### 13.2 部署策略

| 方案 | 说明 |
|------|------|
| **开发环境** | Vite dev server (5173) + proxy → Go backend (8080) |
| **生产环境** | `vite build` → `dist/` 静态文件 → Go backend 通过 `gin.Static()` 嵌入或 Nginx 反向代理 |
| **Docker** | 多阶段构建：Node 阶段 build → 复制 `dist/` 到 Go 容器 |

### 13.3 环境变量

```bash
# .env.development
VITE_API_BASE_URL=http://localhost:8080

# .env.production
VITE_API_BASE_URL=
```

---

## 附录 A: TypeScript 类型定义（与后端 model 对齐）

```ts
// types/models.ts

/** 员工 */
export interface Employee {
  id: number;
  employee_id: string;
  name: string;
  department_id: number;
  department?: string;
  email?: string;
  role: 'employee' | 'approver' | 'admin';
  is_approver: boolean;
  created_at: string;
}

/** 部门 */
export interface Department {
  id: number;
  name: string;
  manager_id?: number;
  manager?: Employee;
  employee_count?: number;
  employees?: Employee[];
  created_at: string;
}

/** 部门预算 */
export interface DepartmentBudget {
  id: number;
  department_id: number;
  department_name: string;
  fiscal_year: number;
  /** 年度预算（元） */
  annual_budget: number;
  /** 已支出（元） */
  spent_amount: number;
  /** 冻结金额（元） */
  frozen_amount: number;
  /** 剩余可用（元） */
  remaining: number;
  /** 使用率 0~1 */
  usage_rate: number;
}

/** 预算看板响应 */
export interface BudgetDashboard {
  year: number;
  departments: DepartmentBudget[];
}

/** 票据项 */
export interface InvoiceItem {
  id: number;
  /** 金额（元） */
  amount: number;
  invoice_date: string;
  category: string;
  check_result: 'pass' | 'warning' | 'error' | 'pending';
}

/** 审批记录 */
export interface ApprovalRecord {
  id: number;
  reimbursement_id: number;
  approver_name: string;
  approver_email?: string;
  action: 'pending' | 'approved' | 'rejected';
  comment?: string;
  action_at?: string;
}

/** 报销单（详情） */
export interface Reimbursement {
  id: number;
  reimbursement_no: string;
  employee_id: string;
  employee_name: string;
  department_id: number;
  department?: string;
  /** 总金额（元） */
  total_amount: number;
  status: 'draft' | 'pending' | 'reviewing' | 'approved' | 'rejected';
  submit_note?: string;
  need_special_approval: boolean;
  invoices: InvoiceItem[];
  approvals: ApprovalRecord[];
  created_at: string;
  updated_at: string;
}

/** 报销单（列表项，不含审批详情） */
export type ReimbursementListItem = Reimbursement;

/** 分页响应 */
export interface PaginatedResponse<T> {
  list: T[];
  total: number;
  page: number;
}
```

```ts
// types/enums.ts

/** 报销单状态 */
export const REIMB_STATUS = {
  draft:     { label: '草稿',    color: 'default' },
  pending:   { label: '待审批',  color: 'processing' },
  reviewing: { label: '审批中',  color: 'warning' },
  approved:  { label: '已通过',  color: 'success' },
  rejected:  { label: '已驳回',  color: 'error' },
} as const;

/** 费用类别 */
export const EXPENSE_CATEGORIES = [
  { value: '差旅-交通', label: '差旅-交通' },
  { value: '差旅-住宿', label: '差旅-住宿' },
  { value: '差旅-补助', label: '差旅-补助' },
  { value: '招待费',    label: '招待费' },
  { value: '办公用品',  label: '办公用品' },
  { value: '印刷费',    label: '印刷费' },
  { value: '其他',      label: '其他' },
] as const;

/** 用户角色 */
export const ROLES = {
  employee: { label: '员工',   color: 'blue' },
  approver: { label: '审批人', color: 'orange' },
  admin:    { label: '管理员', color: 'red' },
} as const;

/** 审批动作 */
export const APPROVAL_ACTIONS = {
  pending:  { label: '待审批' },
  approved: { label: '已通过' },
  rejected: { label: '已驳回' },
} as const;
```

---

## 附录 B: 与后端 SSE 协议的完整对接

| SSE 事件 | 前端组件 | chatStore Action | UI 效果 |
|----------|---------|-----------------|---------|
| `thinking` | `<ThinkingDots />` | `setThinking()` | 三点跳动 + 文字提示 |
| `message` (delta=true) | `<MessageBubble isStreaming>` | `appendStreamContent()` | 打字机逐字追加 + 光标 |
| `message` (delta=false) | `<MessageBubble />` | `finishStreamingMessage()` | 完整消息（替换） |
| `tool_call` | `<ToolCallCard status="running">` | `addToolCall()` | ⏳ 工具执行中卡片 |
| `tool_result` | `<ToolCallCard status="success">` | `updateToolCall()` | ✅ 工具完成卡片 + 折叠结果 |
| `phase_change` | `<PhaseIndicator />` | `setPhase()` | 步骤条高亮切换 |
| `confirm_required` | `<ConfirmModal />` | `setConfirmPrompt()` | 确认弹窗（取消/确认） |
| `error` | antd `message.error()` | — | 错误提示 + 可重试按钮 |
| `done` | — | `finishStreamingMessage()` | 关闭 EventSource + 光标消失 |

---

> **文档状态**: ✅ 已完成，待审核。下一步：搭建脚手架 `pnpm create vite reimbee-web --template react-ts`，进入实施阶段。
