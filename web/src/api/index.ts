import { api } from './client';
import type { PaginatedResponse } from '@/types/models';
import type {
  LoginRequest,
  LoginResponse,
  Department,
  Employee,
  BudgetDashboard,
  DepartmentBudget,
  Reimbursement,
  ApprovalRecord,
} from '@/types/models';

// ============================================
// 认证
// ============================================
export function login(data: LoginRequest) {
  return api.post<LoginResponse>('/api/auth/login', data, { public: true });
}

export function register(data: {
  name: string;
  password: string;
  department_id: number;
}) {
  return api.post<LoginResponse>('/api/auth/register', data, { public: true });
}

// ============================================
// 部门
// ============================================
export function listDepartments(params?: { page?: number; page_size?: number }) {
  return api.get<PaginatedResponse<Department>>('/api/departments', { params });
}

export function getDepartment(id: number) {
  return api.get<Department>(`/api/departments/${id}`);
}

export function createDepartment(data: { name: string; manager_id?: number }) {
  return api.post<Department>('/api/departments', data);
}

export function updateDepartment(id: number, data: { name?: string; manager_id?: number }) {
  return api.put<Department>(`/api/departments/${id}`, data);
}

export function deleteDepartment(id: number) {
  return api.delete<void>(`/api/departments/${id}`);
}

// ============================================
// 员工
// ============================================
export function listEmployees(params?: { page?: number; page_size?: number }) {
  return api.get<PaginatedResponse<Employee>>('/api/employees', { params });
}

export function listApprovers() {
  return api.get<Employee[]>('/api/employees/approvers');
}

export function getEmployee(id: number) {
  return api.get<Employee>(`/api/employees/${id}`);
}

export function createEmployee(data: {
  employee_id: string;
  name: string;
  department_id: number;
  email?: string;
  role?: string;
  is_approver?: boolean;
}) {
  return api.post<Employee>('/api/employees', data);
}

export function updateEmployee(id: number, data: Partial<Employee>) {
  return api.put<Employee>(`/api/employees/${id}`, data);
}

export function deleteEmployee(id: number) {
  return api.delete<void>(`/api/employees/${id}`);
}

// ============================================
// 预算
// ============================================
export function getBudgetDashboard(year?: number) {
  return api.get<BudgetDashboard>('/api/budgets/dashboard', {
    params: { year },
  });
}

export function getBudget(id: number) {
  return api.get<DepartmentBudget>(`/api/budgets/${id}`);
}

export function createBudget(data: {
  department_id: number;
  fiscal_year: number;
  annual_budget: number;
}) {
  return api.post<DepartmentBudget>('/api/budgets', data);
}

export function updateBudget(id: number, data: { annual_budget: number }) {
  return api.put<DepartmentBudget>(`/api/budgets/${id}`, data);
}

// ============================================
// 报销
// ============================================
export function listReimbursements(params?: {
  page?: number;
  page_size?: number;
  employee_id?: string;
}) {
  return api.get<PaginatedResponse<Reimbursement>>('/api/reimbursements', {
    params,
  });
}

export function listPendingReimbursements() {
  return api.get<Reimbursement[]>('/api/reimbursements/pending');
}

export function getReimbursement(id: number) {
  return api.get<Reimbursement>(`/api/reimbursements/${id}`);
}

export function getReimbursementByNo(no: string) {
  return api.get<Reimbursement>(`/api/reimbursements/no/${encodeURIComponent(no)}`);
}

export function createReimbursement(data: {
  employee_id: string;
  employee_name: string;
  department_id: number;
  submit_note?: string;
}) {
  return api.post<Reimbursement>('/api/reimbursements', data);
}

export function submitReimbursement(id: number, total_amount: number) {
  return api.post<Reimbursement>(`/api/reimbursements/${id}/submit`, {
    total_amount,
  });
}

export function approveReimbursement(id: number) {
  return api.post<void>(`/api/reimbursements/${id}/approve`);
}

export function rejectReimbursement(id: number, reason: string) {
  return api.post<void>(`/api/reimbursements/${id}/reject`, { reason });
}

// ============================================
// 审批
// ============================================
export function getApprovalProgress(reimbursementId: number) {
  return api.get<ApprovalRecord[]>(
    `/api/reimbursements/${reimbursementId}/approvals`,
  );
}

export function approveApproval(id: number, comment?: string) {
  return api.post<void>(`/api/approvals/${id}/approve`, { comment });
}

export function rejectApproval(id: number, reason: string) {
  return api.post<void>(`/api/approvals/${id}/reject`, { reason });
}
