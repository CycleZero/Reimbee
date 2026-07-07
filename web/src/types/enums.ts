// 报销单状态
export const REIMB_STATUS: Record<string, { label: string; color: string }> = {
  draft:     { label: '草稿',   color: 'default' },
  pending:   { label: '待审批', color: 'processing' },
  reviewing: { label: '审批中', color: 'warning' },
  approved:  { label: '已通过', color: 'success' },
  rejected:  { label: '已驳回', color: 'error' },
};

// 费用类别
export const EXPENSE_CATEGORIES = [
  { value: '差旅-交通', label: '差旅-交通' },
  { value: '差旅-住宿', label: '差旅-住宿' },
  { value: '差旅-补助', label: '差旅-补助' },
  { value: '招待费',    label: '招待费' },
  { value: '办公用品',  label: '办公用品' },
  { value: '印刷费',    label: '印刷费' },
  { value: '其他',      label: '其他' },
] as const;

// 用户角色
export const ROLES: Record<string, { label: string }> = {
  employee: { label: '员工' },
  approver: { label: '审批人' },
  admin:    { label: '管理员' },
};

// 审批动作
export const APPROVAL_ACTIONS: Record<string, { label: string }> = {
  pending:  { label: '待审批' },
  approved: { label: '已通过' },
  rejected: { label: '已驳回' },
};
