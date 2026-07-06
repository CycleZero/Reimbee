package infra

import (
	"context"
	"io"
)

// UploadedFile 上传的文件元信息
type UploadedFile struct {
	FileID    string `json:"file_id"`    // 文件唯一标识（UUID）
	FileName  string `json:"file_name"`  // 原始文件名
	MimeType  string `json:"mime_type"`  // MIME 类型
	Size      int64  `json:"size"`       // 文件大小（字节）
	URL       string `json:"url"`        // 可访问 URL（/uploads/2026/07/04/xxx.jpg）
	Path      string `json:"path"`       // 存储后端内部路径
	CreatedAt string `json:"created_at"` // 上传时间
}

// FileStorage 文件存储抽象接口
// 所有文件存储后端必须实现此接口，通过配置驱动切换
type FileStorage interface {
	// Save 保存文件，返回文件元信息
	Save(ctx context.Context, fileName string, mimeType string, reader io.Reader) (*UploadedFile, error)

	// Get 根据文件 ID 获取文件内容 Reader（调用方负责 Close）
	Get(ctx context.Context, fileID string) (io.ReadCloser, error)

	// Delete 删除文件
	Delete(ctx context.Context, fileID string) error

	// URL 返回文件的可访问 URL
	URL(ctx context.Context, fileID string) string
}
