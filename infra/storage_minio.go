package infra

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOFileStorage MinIO（S3 兼容）文件存储实现
type MinIOFileStorage struct {
	client     *minio.Client
	bucketName string
	baseURL    string
}

// NewMinIOFileStorage 创建 MinIO 文件存储实例
// 自动检查 Bucket 是否存在，不存在则创建
func NewMinIOFileStorage(endpoint, bucketName, accessKey, secretKey string, useSSL bool, baseURL string) (*MinIOFileStorage, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("连接 MinIO 失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	exists, err := client.BucketExists(ctx, bucketName)
	if err != nil {
		return nil, fmt.Errorf("检查 Bucket 失败: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("创建 Bucket '%s' 失败: %w", bucketName, err)
		}
	}

	return &MinIOFileStorage{
		client:     client,
		bucketName: bucketName,
		baseURL:    baseURL,
	}, nil
}

// Save 保存文件到 MinIO
func (s *MinIOFileStorage) Save(ctx context.Context, fileName string, mimeType string, reader io.Reader) (*UploadedFile, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("读取文件内容失败: %w", err)
	}

	fileID := uuid.New().String()
	now := time.Now()
	ext := filepath.Ext(fileName)
	if ext == "" {
		ext = mimeToExt(mimeType)
	}
	objectName := filepath.Join(now.Format("2006/01/02"), fileID+ext)

	_, err = s.client.PutObject(ctx, s.bucketName, objectName,
		bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: mimeType},
	)
	if err != nil {
		return nil, fmt.Errorf("上传文件到 MinIO 失败: %w", err)
	}

	return &UploadedFile{
		FileID:    fileID,
		FileName:  fileName,
		MimeType:  mimeType,
		Size:      int64(len(data)),
		URL:       fmt.Sprintf("%s/%s/%s", s.baseURL, s.bucketName, objectName),
		Path:      objectName,
		CreatedAt: now.Format(time.RFC3339),
	}, nil
}

// Get 从 MinIO 读取文件
func (s *MinIOFileStorage) Get(ctx context.Context, objectName string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("从 MinIO 读取文件失败: %w", err)
	}
	return obj, nil
}

// Delete 从 MinIO 删除文件
func (s *MinIOFileStorage) Delete(ctx context.Context, objectName string) error {
	return s.client.RemoveObject(ctx, s.bucketName, objectName, minio.RemoveObjectOptions{})
}

// URL 返回 MinIO 文件访问 URL
func (s *MinIOFileStorage) URL(_ context.Context, objectName string) string {
	return fmt.Sprintf("%s/%s/%s", s.baseURL, s.bucketName, objectName)
}

// Client 返回原始 MinIO Client（供测试使用）
func (s *MinIOFileStorage) Client() *minio.Client { return s.client }

// BucketName 返回 Bucket 名称
func (s *MinIOFileStorage) BucketName() string { return s.bucketName }
