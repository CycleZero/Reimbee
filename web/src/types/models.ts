// ============================================
// 前端数据模型 —— 与 API 文档（默认模块0707.md）严格对齐
// 金额单位：API 返回分（int），前端 formatAmount() 转为元（float）展示
// ============================================

// ============================================
// 认证
// ============================================

export interface LoginRequest {
  employee_id: string;
  password: string;
}

/** 登录成功返回 — JWT + 用户基本信息 */
export interface LoginResponse {
  token: string;
  employee_id: string;
  name: string;
  role: 'employee' | 'approver' | 'admin';
  expires_in: number;
}

export interface RegisterRequest {
  department_id: number;
  email?: string;
  name: string;
  password: string;
}

export interface RegisterResponse {
  employee_id: string;
  message: string;
  name: string;
  role: string;
}

// ============================================
// 用户身份（authStore 存储的轻量用户信息）
// ============================================

export interface UserInfo {
  employee_id: string;
  name: string;
  role: 'employee' | 'approver' | 'admin';
  department_id?: number;
  is_approver?: boolean;
}

// ============================================
// 部门
// ============================================

export interface Department {
  id: number;
  name: string;
  manager_id?: number;
  created_at: string;
  updated_at: string;
}

export interface CreateDepartmentRequest {
  name: string;
  manager_id?: number;
}

export interface UpdateDepartmentRequest {
  name: string;
  manager_id?: number;
}

// ============================================
// 员工
// ============================================

export interface Employee {
  id: number;
  employee_id: string;
  name: string;
  department_id: number;
  department: string; // 部门名称（后端 join 填充）
  email?: string;
  role: 'employee' | 'approver' | 'admin';
  is_approver: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateEmployeeRequest {
  employee_id: string;
  name: string;
  department_id: number;
  email?: string;
  role?: string;
}

export interface UpdateEmployeeRequest {
  name: string;
  department_id: number;
  email?: string;
  role?: string;
}

// ============================================
// 预算
// ============================================

export interface DepartmentBudget {
  id: number;
  department_id: number;
  department: string; // 部门名称
  fiscal_year: number;
  annual_budget: number; // 分
  spent_amount: number; // 分
  frozen_amount: number; // 分
  remaining: number; // 分
  usage_rate: number; // 0~1
  created_at: string;
  updated_at: string;
}

export interface BudgetDashboardSummary {
  total_budget: number; // 分
  total_spent: number; // 分
  total_remaining: number; // 分
  overall_usage: number; // 0~1
}

export interface BudgetDashboard {
  departments: DepartmentBudget[];
  summary: BudgetDashboardSummary;
}

export interface CreateBudgetRequest {
  department_id: number;
  fiscal_year: number;
  annual_budget: number; // 分
}

export interface UpdateBudgetRequest {
  annual_budget: number; // 分
}

// ============================================
// 报销
// ============================================

export interface InvoiceItem {
  id: number;
  amount: number; // 分
  category: string;
  check_result: 'pass' | 'warning' | 'error' | 'pending';
  invoice_date: string;
}

/** 报销单中内嵌的审批信息（简化版） */
export interface ApprovalInfo {
  id: number;
  action: 'pending' | 'approved' | 'rejected';
  action_at?: string;
  approver_name: string;
  comment?: string;
}

export interface Reimbursement {
  id: number;
  reimbursement_no: string;
  employee_id: string;
  employee_name: string;
  department_id: number;
  department: string; // 部门名称
  total_amount: number; // 分
  status: 'draft' | 'pending' | 'reviewing' | 'approved' | 'rejected';
  submit_note?: string;
  need_special_approval: boolean;
  invoices: InvoiceItem[];
  approvals: ApprovalInfo[];
  created_at: string;
  updated_at: string;
}

export interface CreateReimbursementRequest {
  employee_id: string;
  employee_name: string;
  department_id: number;
  submit_note?: string;
}

export interface SubmitReimbursementRequest {
  total_amount: number; // 分
}

// ============================================
// 审批
// ============================================

/** 审批记录详情（审批进度接口返回） */
export interface ApprovalRecord {
  id: number;
  reimbursement_id: number;
  approver_name: string;
  approver_email?: string;
  action: 'pending' | 'approved' | 'rejected';
  comment?: string;
  action_at?: string;
  created_at: string;
  updated_at: string;
}

// ============================================
// 公共
// ============================================

export interface PaginatedResponse<T> {
  list: T[];
  total: number;
  page: number;
}

// ============================================
// 文件上传
// ============================================

/** 票据上传响应 — 与后端 reimbursement/dto.go UploadInvoiceResponse 对齐 */
export interface UploadInvoiceResponse {
  file_id: string;
  file_name: string;
  file_path: string;
  url: string;
  size: number;
}

// ============================================
// 知识库管理
// ============================================

export interface PolicyDocument {
  id: number;
  title: string;
  version: string;
  effective_date: string;
  status: string;
  chunk_count: number;
  created_at: string;
  updated_at: string;
}

export interface PolicyDocumentDetail extends PolicyDocument {
  content: string;
  chunks: PolicyChunk[];
}

export interface PolicyChunk {
  id: number;
  chunk_index: number;
  content: string;
}

export interface CreatePolicyRequest {
  title: string;
  content: string;
  version?: string;
  effective_date?: string;
}

export interface UpdatePolicyRequest {
  title: string;
  content: string;
  version: string;
  effective_date: string;
  status: string;
}

export interface KnowledgeBaseStatus {
  document_count: number;
  chunk_count: number;
  search_mode: string;
  embedder_model?: string;
  vector_store?: string;
  healthy: boolean;
}

export interface SearchTestResult {
  query: string;
  mode: string;
  chunks: SearchTestChunk[];
}

export interface SearchTestChunk {
  document_id: number;
  document_title: string;
  chunk_index: number;
  content: string;
  score?: number;
}
