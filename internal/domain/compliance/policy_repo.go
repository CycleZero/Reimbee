package compliance

import (
	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/model"
	"gorm.io/gorm"
)

// PolicyRepo 政策文档数据访问层
type PolicyRepo struct {
	db *gorm.DB
}

// NewPolicyRepo 创建政策文档数据访问层实例
func NewPolicyRepo(data *infra.Data) *PolicyRepo {
	return &PolicyRepo{db: data.DB}
}

// List 分页查询文档列表（不含 chunks，减少传输量）
func (r *PolicyRepo) List(page, pageSize int) ([]*model.PolicyDocument, int64, error) {
	var docs []*model.PolicyDocument
	var total int64

	if err := r.db.Model(&model.PolicyDocument{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.Offset((page - 1) * pageSize).Limit(pageSize).
		Order("id DESC").Find(&docs).Error
	return docs, total, err
}

// GetByID 查询单个文档（含分块列表）
func (r *PolicyRepo) GetByID(id uint) (*model.PolicyDocument, error) {
	var doc model.PolicyDocument
	if err := r.db.Preload("Chunks").First(&doc, id).Error; err != nil {
		return nil, err
	}
	return &doc, nil
}

// Update 更新文档元数据
func (r *PolicyRepo) Update(doc *model.PolicyDocument) error {
	return r.db.Save(doc).Error
}

// Delete 删除文档（CASCADE 自动删关联 chunks）
func (r *PolicyRepo) Delete(id uint) error {
	return r.db.Delete(&model.PolicyDocument{}, id).Error
}

// CountDocuments 统计文档总数
func (r *PolicyRepo) CountDocuments() (int64, error) {
	var count int64
	err := r.db.Model(&model.PolicyDocument{}).Count(&count).Error
	return count, err
}

// CountChunks 统计分块总数
func (r *PolicyRepo) CountChunks() (int64, error) {
	var count int64
	err := r.db.Model(&model.PolicyChunk{}).Count(&count).Error
	return count, err
}
