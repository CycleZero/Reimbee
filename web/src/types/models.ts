// ============================================
// 前端数据模型 —— 与后端 model/*.go 对齐
// 金额单位：元（float64），后端存储为分（int64）
// ============================================

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

export interface Department {
  id: number;
  name: string;
  manager_id?: number;
  manager?: Employee;
  employee_count?: number;
  employees?: Employee[];
  created_at: string;
}

export interface DepartmentBudget {
  id: number;
  department_id: number;
  department_name: string;
  fiscal_year: number;
  annual_budget: number;
  spent_amount: number;
  frozen_amount: number;
  remaining: number;
  usage_rate: number;
}

export interface BudgetDashboard {
  year: number;
  departments: DepartmentBudget[];
}

export interface InvoiceItem {
  id: number;
  amount: number;
  invoice_date: string;
  category: string;
  check_result: 'pass' | 'warning' | 'error' | 'pending';
}

export interface ApprovalRecord {
  id: number;
  reimbursement_id: number;
  approver_name: string;
  approver_email?: string;
  action: 'pending' | 'approved' | 'rejected';
  comment?: string;
  action_at?: string;
}

export interface Reimbursement {
  id: number;
  reimbursement_no: string;
  employee_id: string;
  employee_name: string;
  department_id: number;
  department?: string;
  total_amount: number;
  status: 'draft' | 'pending' | 'reviewing' | 'approved' | 'rejected';
  submit_note?: string;
  need_special_approval: boolean;
  invoices: InvoiceItem[];
  approvals: ApprovalRecord[];
  created_at: string;
  updated_at: string;
}

export type ReimbursementListItem = Reimbursement;

export interface PaginatedResponse<T> {
  list: T[];
  total: number;
  page: number;
}

export interface LoginRequest {
  employee_id: string;
  password: string;
}

export interface LoginResponse {
  token: string;
  employee: Employee;
}
