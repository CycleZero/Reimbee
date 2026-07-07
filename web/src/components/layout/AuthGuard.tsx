import { Navigate, useLocation } from 'react-router-dom';
import { useAuthStore } from '@/stores/authStore';
import type { ReactNode } from 'react';

type Role = 'employee' | 'approver' | 'admin';

const ROUTE_ROLE: Record<string, Role> = {
  '/reimbursements/pending': 'approver',
  '/employees': 'approver',
  '/departments': 'admin',
  '/budgets': 'admin',
};

interface Props {
  children: ReactNode;
}

export function AuthGuard({ children }: Props) {
  const { user, isAuthenticated } = useAuthStore();
  const location = useLocation();

  if (!isAuthenticated || !user) {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }

  const required = Object.entries(ROUTE_ROLE).find(([prefix]) =>
    location.pathname.startsWith(prefix),
  );

  if (required) {
    const roles: Role[] = ['employee', 'approver', 'admin'];
    const userIdx = roles.indexOf(user.role as Role);
    const needIdx = roles.indexOf(required[1]);
    if (userIdx < needIdx) {
      return (
        <div style={{ padding: 48, textAlign: 'center' }}>
          <h2>403 — 无权限访问</h2>
          <p>您没有访问此页面的权限</p>
        </div>
      );
    }
  }

  return <>{children}</>;
}
