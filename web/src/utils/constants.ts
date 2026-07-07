// 工具名称中文映射（SSE tool_call/tool_result 展示用）
export const TOOL_LABELS: Record<string, string> = {
  recognize_invoice: '票据识别',
  check_compliance: '合规检查',
  check_budget: '预算检查',
  generate_pdf: '生成 PDF',
  send_email: '发送邮件',
  query_progress: '查询进度',
  query_records: '查询记录',
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
