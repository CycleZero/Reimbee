package model

import "gorm.io/gorm"

// PolicyDocument 合规政策文档
type PolicyDocument struct {
	gorm.Model
	Title         string `gorm:"type:varchar(200);not null;comment:文档标题" json:"title"`
	Content       string `gorm:"type:longtext;not null;comment:原始文档内容" json:"content"`
	Version       string `gorm:"type:varchar(20);not null;comment:版本号" json:"version"`
	EffectiveDate string `gorm:"type:varchar(20);not null;comment:生效日期" json:"effective_date"`
	Status        string `gorm:"type:varchar(20);default:active;comment:active/archived" json:"status"`
	Chunks        []PolicyChunk `gorm:"foreignKey:DocumentID" json:"chunks,omitempty"`
}

// PolicyChunk 文档分块(含向量嵌入)
type PolicyChunk struct {
	gorm.Model
	DocumentID uint   `gorm:"index;not null;comment:关联文档ID" json:"document_id"`
	ChunkIndex int    `gorm:"not null;comment:分块序号" json:"chunk_index"`
	Content    string `gorm:"type:text;not null;comment:分块文本内容" json:"content"`
	Embedding  string `gorm:"type:mediumtext;comment:向量嵌入JSON(float64数组)" json:"embedding,omitempty"`
}
