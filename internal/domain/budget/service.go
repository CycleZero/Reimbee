package budget

import (
	"net/http"
	"strconv"

	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const defaultFiscalYear = 2026

// BudgetService 预算 HTTP 服务层
type BudgetService struct {
	biz    *BudgetBiz
	logger *log.Logger
}

// NewBudgetService 创建预算 HTTP 服务层实例
func NewBudgetService(biz *BudgetBiz, logger *log.Logger) *BudgetService {
	return &BudgetService{biz: biz, logger: logger}
}

// Dashboard 获取当前财年的预算看板
// @Summary 获取预算看板
// @Description 获取指定财年所有部门的预算概览，包含汇总统计
// @Tags 预算管理
// @Accept json
// @Produce json
// @Param year query int false "财年，默认2026"
// @Success 200 {object} DashboardResponse "预算看板数据"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/budgets/dashboard [get]
func (s *BudgetService) Dashboard(c *gin.Context) {
	year := defaultFiscalYear
	if y := c.Query("year"); y != "" {
		if parsed, err := strconv.Atoi(y); err == nil {
			year = parsed
		}
	}

	budgets, err := s.biz.GetDashboard(year)
	if err != nil {
		s.logger.Error("获取预算看板失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取预算看板失败"})
		return
	}

	departments := make([]*BudgetResponse, 0, len(budgets))
	var totalBudget, totalSpent, totalRemaining int64
	for _, b := range budgets {
		resp := toBudgetResponse(b)
		departments = append(departments, resp)
		totalBudget += b.AnnualBudget
		totalSpent += b.SpentAmount
		totalRemaining += resp.Remaining
	}

	var overallUsage float64
	if totalBudget > 0 {
		overallUsage = float64(totalSpent) / float64(totalBudget) * 100
	}

	c.JSON(http.StatusOK, DashboardResponse{
		Departments: departments,
		Summary: DashboardSummary{
			TotalBudget:    totalBudget,
			TotalSpent:     totalSpent,
			TotalRemaining: totalRemaining,
			OverallUsage:   overallUsage,
		},
	})
}

// GetByID 根据ID获取预算详情
// @Summary 获取预算详情
// @Description 根据预算ID获取单条预算记录
// @Tags 预算管理
// @Accept json
// @Produce json
// @Param id path int true "预算ID"
// @Success 200 {object} BudgetResponse "预算详情"
// @Failure 400 {object} map[string]interface{} "预算ID格式错误"
// @Failure 404 {object} map[string]interface{} "预算记录不存在"
// @Router /api/budgets/{id} [get]
func (s *BudgetService) GetByID(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "预算ID格式错误"})
		return
	}

	budget, err := s.biz.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "预算记录不存在"})
		return
	}
	c.JSON(http.StatusOK, toBudgetResponse(budget))
}

// Create 创建预算记录
// @Summary 创建预算记录
// @Description 管理员为指定部门创建财年预算
// @Tags 预算管理
// @Accept json
// @Produce json
// @Param request body CreateBudgetRequest true "创建预算请求"
// @Success 201 {object} BudgetResponse "预算创建成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 409 {object} map[string]interface{} "该部门该财年预算已存在"
// @Router /api/budgets [post]
func (s *BudgetService) Create(c *gin.Context) {
	var req CreateBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	budget, err := s.biz.Create(req.DepartmentID, req.FiscalYear, req.AnnualBudget)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, toBudgetResponse(budget))
}

// Update 更新预算
// @Summary 更新预算金额
// @Description 管理员更新指定预算记录的年度预算金额
// @Tags 预算管理
// @Accept json
// @Produce json
// @Param id path int true "预算ID"
// @Param request body UpdateBudgetRequest true "更新预算请求"
// @Success 200 {object} BudgetResponse "预算更新成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 409 {object} map[string]interface{} "预算更新失败"
// @Router /api/budgets/{id} [put]
func (s *BudgetService) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "预算ID格式错误"})
		return
	}

	var req UpdateBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	budget, err := s.biz.Update(uint(id), req.AnnualBudget)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toBudgetResponse(budget))
}

// toBudgetResponse 将模型转换为响应 DTO
func toBudgetResponse(b *model.DepartmentBudget) *BudgetResponse {
	remaining := b.AnnualBudget - b.SpentAmount - b.FrozenAmount
	var usageRate float64
	if b.AnnualBudget > 0 {
		usageRate = float64(b.SpentAmount) / float64(b.AnnualBudget) * 100
	}

	resp := &BudgetResponse{
		ID:           b.ID,
		DepartmentID: b.DepartmentID,
		FiscalYear:   b.FiscalYear,
		AnnualBudget: b.AnnualBudget,
		SpentAmount:  b.SpentAmount,
		FrozenAmount: b.FrozenAmount,
		Remaining:    remaining,
		UsageRate:    usageRate,
		CreatedAt:    b.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:    b.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
	if b.Department != nil {
		resp.Department = b.Department.Name
	}
	return resp
}
