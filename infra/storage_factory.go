package infra

import (
	"fmt"

	"github.com/spf13/viper"
)

// NewFileStorage 根据配置创建对应的文件存储实例
// 通过 config.yaml 中的 storage.driver 字段切换:
//   - "local": 本地磁盘（默认）
//   - "minio": MinIO / S3 兼容对象存储
func NewFileStorage(vc *viper.Viper) (FileStorage, error) {
	driver := vc.GetString("storage.driver")
	if driver == "" {
		driver = "local"
	}

	switch driver {
	case "local":
		baseDir := vc.GetString("storage.local.base_dir")
		if baseDir == "" {
			baseDir = "./uploads"
		}
		baseURL := vc.GetString("storage.local.base_url")
		if baseURL == "" {
			baseURL = "/uploads"
		}
		return NewLocalFileStorage(baseDir, baseURL), nil

	case "minio":
		return NewMinIOFileStorage(
			vc.GetString("storage.minio.endpoint"),
			vc.GetString("storage.minio.bucket"),
			vc.GetBool("storage.minio.use_ssl"),
			vc.GetString("storage.minio.base_url"),
		), nil

	default:
		return nil, fmt.Errorf("未知存储驱动: %s (可选: local, minio)", driver)
	}
}
