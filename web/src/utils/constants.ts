// 工具名称中文映射（21 个工具 + 系统卡片标签）
export const TOOL_LABELS: Record<string, string> = {
  // 员工专属工具
  recognize_invoice: '票据识别',
  check_budget: '预算检查',
  get_department_id: '部门查询',
  create_reimbursement: '创建报销单',
  submit_reimbursement: '提交报销',
  cancel_reimbursement: '取消草稿',
  generate_pdf: '生成 PDF',
  send_email: '发送邮件',
  // 审批人专属工具
  list_pending: '待审批列表',
  approve_reimbursement: '审批通过',
  reject_reimbursement: '驳回',
  // 共享工具
  search_policy: '政策检索',
  check_compliance: '合规审核',
  list_invoices: '票据汇总',
  check_deadline: '有效期校验',
  query_reimbursements: '报销记录',
  get_reimbursement_detail: '报销详情',
  query_progress: '审批进度',
  search_department: '搜索部门',
  search_employee: '搜索员工',
  test_interrupt: '中断测试',
  // 系统卡片标签
  thinking: '思考中',
  error: '错误',
};

// 角色中文名
export const ROLE_LABELS: Record<string, string> = {
  employee: '员工',
  approver: '审批人',
  admin: '管理员',
};

// API 错误码中文映射
export const ERROR_MESSAGES: Record<string, string> = {
  ocr_error: '票据识别失败，请检查图片是否清晰后重试',
  budget_insufficient: '部门预算不足，请联系管理员',
  no_approver: '未配置审批人，请联系管理员',
  status_not_allowed: '当前状态不允许此操作',
  department_has_employees: '该部门下有关联员工，无法删除',
  budget_duplicate: '该部门同一年度已有预算',
  employee_duplicate: '该工号已存在',
  stream_error: '对话连接中断，正在尝试重连...',
};
