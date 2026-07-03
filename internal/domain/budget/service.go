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

// GetByID 根据 ID 获取预算详情
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
