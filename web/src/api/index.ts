import { api } from './client';
import type {
  LoginRequest,
  LoginResponse,
  RegisterRequest,
  RegisterResponse,
  Department,
  CreateDepartmentRequest,
  UpdateDepartmentRequest,
  Employee,
  CreateEmployeeRequest,
  UpdateEmployeeRequest,
  BudgetDashboard,
  DepartmentBudget,
  CreateBudgetRequest,
  UpdateBudgetRequest,
  Reimbursement,
  CreateReimbursementRequest,
  SubmitReimbursementRequest,
  ApprovalRecord,
  PaginatedResponse,
} from '@/types/models';

// ============================================
// 认证
// ============================================

/** 用户登录，返回 JWT + 用户基本信息 */
export function login(data: LoginRequest) {
  return api.post<LoginResponse>('/api/auth/login', data, { public: true });
}

/** 用户注册 */
export function register(data: RegisterRequest) {
  return api.post<RegisterResponse>('/api/auth/register', data, { public: true });
}

// ============================================
// 部门管理
// ============================================

/** 分页获取部门列表 */
export function listDepartments(params?: { page?: number; page_size?: number }) {
  return api.get<PaginatedResponse<Department>>('/api/departments', { params });
}

/** 获取单个部门详情 */
export function getDepartment(id: number) {
  return api.get<Department>(`/api/departments/${id}`);
}

/** 管理员创建新部门 */
export function createDepartment(data: CreateDepartmentRequest) {
  return api.post<Department>('/api/departments', data);
}

/** 管理员更新部门信息 */
export function updateDepartment(id: number, data: UpdateDepartmentRequest) {
  return api.put<Department>(`/api/departments/${id}`, data);
}

/** 管理员删除部门 */
export function deleteDepartment(id: number) {
  return api.delete<void>(`/api/departments/${id}`);
}

// ============================================
// 员工管理
// ============================================

/** 分页获取员工列表 */
export function listEmployees(params?: { page?: number; page_size?: number }) {
  return api.get<PaginatedResponse<Employee>>('/api/employees', { params });
}

/** 获取审批人列表 */
export function listApprovers() {
  return api.get<Employee[]>('/api/employees/approvers');
}

/** 获取单个员工详情 */
export function getEmployee(id: number) {
  return api.get<Employee>(`/api/employees/${id}`);
}

/** 管理员创建新员工 */
export function createEmployee(data: CreateEmployeeRequest) {
  return api.post<Employee>('/api/employees', data);
}

/** 管理员更新员工信息 */
export function updateEmployee(id: number, data: UpdateEmployeeRequest) {
  return api.put<Employee>(`/api/employees/${id}`, data);
}

/** 管理员删除员工 */
export function deleteEmployee(id: number) {
  return api.delete<void>(`/api/employees/${id}`);
}

// ============================================
// 预算管理
// ============================================

/** 获取预算看板（含汇总统计） */
export function getBudgetDashboard(year?: number) {
  return api.get<BudgetDashboard>('/api/budgets/dashboard', { params: { year } });
}

/** 获取单条预算记录 */
export function getBudget(id: number) {
  return api.get<DepartmentBudget>(`/api/budgets/${id}`);
}

/** 管理员创建部门预算 */
export function createBudget(data: CreateBudgetRequest) {
  return api.post<DepartmentBudget>('/api/budgets', data);
}

/** 管理员更新预算金额 */
export function updateBudget(id: number, data: UpdateBudgetRequest) {
  return api.put<DepartmentBudget>(`/api/budgets/${id}`, data);
}

// ============================================
// 报销管理
// ============================================

/** 分页获取报销单列表（支持按工号筛选） */
export function listReimbursements(params?: {
  page?: number;
  page_size?: number;
  employee_id?: string;
}) {
  return api.get<PaginatedResponse<Reimbursement>>('/api/reimbursements', { params });
}

/** 获取待审批报销单列表（审批人专用） */
export function listPendingReimbursements() {
  return api.get<Reimbursement[]>('/api/reimbursements/pending');
}

/** 按 ID 获取报销单详情 */
export function getReimbursement(id: number) {
  return api.get<Reimbursement>(`/api/reimbursements/${id}`);
}

/** 按单号获取报销单详情 */
export function getReimbursementByNo(no: string) {
  return api.get<Reimbursement>(`/api/reimbursements/no/${encodeURIComponent(no)}`);
}

/** 员工创建报销单草稿 */
export function createReimbursement(data: CreateReimbursementRequest) {
  return api.post<Reimbursement>('/api/reimbursements', data);
}

/** 员工提交报销单进入审批流程 */
export function submitReimbursement(id: number, data: SubmitReimbursementRequest) {
  return api.post<Reimbursement>(`/api/reimbursements/${id}/submit`, data);
}

/** 审批人通过报销单 */
export function approveReimbursement(id: number) {
  return api.post<Reimbursement>(`/api/reimbursements/${id}/approve`);
}

/** 审批人驳回报销单 */
export function rejectReimbursement(id: number, reason: string) {
  return api.post<Reimbursement>(`/api/reimbursements/${id}/reject`, { reason });
}

// ============================================
// 审批记录
// ============================================

/** 获取报销单的审批流转记录 */
export function getApprovalProgress(reimbursementId: number) {
  return api.get<ApprovalRecord[]>(`/api/reimbursements/${reimbursementId}/approvals`);
}

/** 审批人通过审批记录 */
export function approveApproval(id: number, comment?: string) {
  return api.post<void>(`/api/approvals/${id}/approve`, comment ? { comment } : {});
}

/** 审批人驳回审批记录 */
export function rejectApproval(id: number, reason: string) {
  return api.post<void>(`/api/approvals/${id}/reject`, { reason });
}
