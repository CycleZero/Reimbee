package model

import "gorm.io/gorm"

// Demo 示例模型 - 展示 GORM 模型定义
// 替换或删除此模型以定义你自己的业务模型
type Demo struct {
	gorm.Model
	Name        string `gorm:"type:varchar(100);not null;comment:名称" json:"name"`
	Description string `gorm:"type:text;comment:描述" json:"description"`
	Status      int    `gorm:"type:tinyint;default:1;comment:状态 1=正常 0=禁用" json:"status"`
	CreatedBy   uint   `gorm:"comment:创建人ID" json:"created_by"`
}
