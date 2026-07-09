import { createBrowserRouter, Navigate } from 'react-router-dom';
import { lazy, Suspense } from 'react';
import { Spin } from 'antd';

// 布局（非懒加载）
import { AppLayout } from '@/components/layout/AppLayout';

// 页面懒加载
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
const PolicyManage = lazy(() => import('@/pages/PolicyManage'));

function PageLoader() {
  return (
    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: 200 }}>
      <Spin size="large" />
    </div>
  );
}

function Lazy({ children }: { children: React.ReactNode }) {
  return <Suspense fallback={<PageLoader />}>{children}</Suspense>;
}

export const router = createBrowserRouter([
  {
    path: '/login',
    element: (
      <Lazy>
        <Login />
      </Lazy>
    ),
  },
  {
    path: '/',
    element: <AppLayout />,
    children: [
      { index: true, element: <Navigate to="/dashboard" replace /> },
      {
        path: 'dashboard',
        element: <Lazy><Dashboard /></Lazy>,
      },
      {
        path: 'reimbursements',
        element: <Lazy><ReimbursementList /></Lazy>,
      },
      {
        path: 'reimbursements/create',
        element: <Lazy><ReimbursementCreate /></Lazy>,
      },
      {
        path: 'reimbursements/pending',
        element: <Lazy><PendingApprovals /></Lazy>,
      },
      {
        path: 'reimbursements/:id',
        element: <Lazy><ReimbursementDetail /></Lazy>,
      },
      {
        path: 'employees',
        element: <Lazy><EmployeeList /></Lazy>,
      },
      {
        path: 'departments',
        element: <Lazy><DepartmentList /></Lazy>,
      },
      {
        path: 'budgets',
        element: <Lazy><BudgetManage /></Lazy>,
      },
      {
        path: 'chat',
        element: <Lazy><Chat /></Lazy>,
      },
      {
        path: 'chat/:sessionId',
        element: <Lazy><Chat /></Lazy>,
      },
      {
        path: 'policies',
        element: <Lazy><PolicyManage /></Lazy>,
      },
      { path: '*', element: <div style={{ padding: 48, textAlign: 'center' }}><h2>404</h2></div> },
    ],
  },
]);
