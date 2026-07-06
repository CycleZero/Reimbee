package infra

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// minioTestConfig 从环境变量读取 MinIO 连接信息（凭据不入库）
func minioTestConfig() (endpoint, accessKey, secretKey, bucket, baseURL string, useSSL bool) {
	endpoint = os.Getenv("MINIO_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:9000"
	}
	accessKey = os.Getenv("MINIO_ACCESS_KEY")
	if accessKey == "" {
		accessKey = "minioadmin"
	}
	secretKey = os.Getenv("MINIO_SECRET_KEY")
	if secretKey == "" {
		secretKey = "minioadmin"
	}
	bucket = os.Getenv("MINIO_BUCKET")
	if bucket == "" {
		bucket = "reimbee"
	}
	baseURL = os.Getenv("MINIO_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:9000"
	}
	useSSL = os.Getenv("MINIO_USE_SSL") == "true"
	return
}

// setupMinIO 创建测试用 MinIO 存储实例
func setupMinIO(t *testing.T) *MinIOFileStorage {
	t.Helper()
	endpoint, accessKey, secretKey, bucket, baseURL, useSSL := minioTestConfig()
	store, err := NewMinIOFileStorage(endpoint, bucket, accessKey, secretKey, useSSL, baseURL)
	if err != nil {
		t.Fatalf("连接 MinIO 失败: %v", err)
	}
	t.Logf("MinIO 连接成功, Bucket: %s", store.BucketName())
	return store
}

func TestMinIOStorage_Constructor_BucketCreation(t *testing.T) {
	store := setupMinIO(t)

	// 验证 Bucket 存在
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exists, err := store.client.BucketExists(ctx, store.BucketName())
	if err != nil {
		t.Fatalf("检查 Bucket 存在性失败: %v", err)
	}
	if !exists {
		t.Fatalf("Bucket '%s' 应存在但不存在", store.BucketName())
	}
	t.Logf("Bucket '%s' 存在确认", store.BucketName())
}

func TestMinIOStorage_SaveJPEG(t *testing.T) {
	store := setupMinIO(t)
	ctx := context.Background()

	// 模拟一张 1x1 像素的 JPEG
	fakeJPEG := []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46,
		0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01,
		0x00, 0x01, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
		0x00, 0xFF, 0xD9,
	}

	result, err := store.Save(ctx, "test-invoice.jpg", "image/jpeg", bytes.NewReader(fakeJPEG))
	if err != nil {
		t.Fatalf("保存 JPEG 失败: %v", err)
	}

	// 验证返回的元信息
	if result.FileID == "" {
		t.Error("FileID 不应为空")
	}
	if result.FileName != "test-invoice.jpg" {
		t.Errorf("FileName 应为 'test-invoice.jpg', 实际 '%s'", result.FileName)
	}
	if result.MimeType != "image/jpeg" {
		t.Errorf("MimeType 应为 'image/jpeg', 实际 '%s'", result.MimeType)
	}
	if result.Size != int64(len(fakeJPEG)) {
		t.Errorf("Size 应为 %d, 实际 %d", len(fakeJPEG), result.Size)
	}
	if result.Path == "" {
		t.Error("Path 不应为空")
	}
	if result.URL == "" {
		t.Error("URL 不应为空")
	}
	if !strings.Contains(result.Path, ".jpg") {
		t.Errorf("Path 应包含扩展名 .jpg, 实际: %s", result.Path)
	}
	if !strings.HasPrefix(result.Path, "2026/") {
		t.Errorf("Path 应以日期前缀开头, 实际: %s", result.Path)
	}

	t.Logf("保存成功: FileID=%s, Path=%s, Size=%d, URL=%s",
		result.FileID, result.Path, result.Size, result.URL)

	// 清理
	defer store.Delete(ctx, result.Path)
}

func TestMinIOStorage_SavePNG(t *testing.T) {
	store := setupMinIO(t)
	ctx := context.Background()

	fakePNG := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x49, 0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
	}

	result, err := store.Save(ctx, "receipt.png", "image/png", bytes.NewReader(fakePNG))
	if err != nil {
		t.Fatalf("保存 PNG 失败: %v", err)
	}
	if result.MimeType != "image/png" {
		t.Errorf("MimeType 应为 'image/png', 实际 '%s'", result.MimeType)
	}
	t.Logf("PNG 保存成功: Path=%s", result.Path)

	defer store.Delete(ctx, result.Path)
}

func TestMinIOStorage_SavePDF(t *testing.T) {
	store := setupMinIO(t)
	ctx := context.Background()

	fakePDF := []byte("%PDF-1.4 test content")

	result, err := store.Save(ctx, "电子发票.pdf", "application/pdf", bytes.NewReader(fakePDF))
	if err != nil {
		t.Fatalf("保存 PDF 失败: %v", err)
	}
	if result.MimeType != "application/pdf" {
		t.Errorf("MimeType 应为 'application/pdf', 实际 '%s'", result.MimeType)
	}
	t.Logf("PDF 保存成功: Path=%s", result.Path)

	defer store.Delete(ctx, result.Path)
}

func TestMinIOStorage_Get(t *testing.T) {
	store := setupMinIO(t)
	ctx := context.Background()

	// 先上传
	content := []byte("测试文件内容——这是一张发票图片的模拟数据")
	result, err := store.Save(ctx, "test-get.txt", "text/plain", bytes.NewReader(content))
	if err != nil {
		t.Fatalf("保存测试文件失败: %v", err)
	}
	defer store.Delete(ctx, result.Path)

	// 再读取
	reader, err := store.Get(ctx, result.Path)
	if err != nil {
		t.Fatalf("读取文件失败: %v", err)
	}
	defer reader.Close()

	readBack, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("读取内容失败: %v", err)
	}

	if !bytes.Equal(content, readBack) {
		t.Errorf("读取内容与原始内容不一致\n原始: %s\n读取: %s", content, readBack)
	}
	t.Logf("读取验证成功: 原始 %d bytes = 读取 %d bytes", len(content), len(readBack))
}

func TestMinIOStorage_GetNonExistent(t *testing.T) {
	store := setupMinIO(t)
	ctx := context.Background()

	reader, err := store.Get(ctx, "nonexistent/file.txt")
	if err != nil {
		t.Logf("读取不存在文件正确返回错误(立即): %v", err)
		return
	}
	defer reader.Close()

	// MinIO SDK 的 GetObject 可能不立即报错，读到数据时才报错
	_, readErr := io.ReadAll(reader)
	if readErr != nil {
		t.Logf("读取不存在文件正确返回错误(延迟): %v", readErr)
		return
	}
	t.Fatal("读取不存在的文件应返回错误，但成功读取了数据")
}

func TestMinIOStorage_Delete(t *testing.T) {
	store := setupMinIO(t)
	ctx := context.Background()

	// 上传
	content := []byte("待删除的临时文件")
	result, err := store.Save(ctx, "to-delete.txt", "text/plain", bytes.NewReader(content))
	if err != nil {
		t.Fatalf("保存文件失败: %v", err)
	}

	// 删除
	if err := store.Delete(ctx, result.Path); err != nil {
		t.Fatalf("删除文件失败: %v", err)
	}

	// 确认已删除：MinIO GetObject 可能不立即报错，读到数据时才报错
	reader, err := store.Get(ctx, result.Path)
	if err != nil {
		t.Log("删除验证成功：Get 立即返回错误")
		return
	}
	defer reader.Close()
	_, readErr := io.ReadAll(reader)
	if readErr != nil {
		t.Logf("删除验证成功：读取报错 '%v'", readErr)
		return
	}
	t.Fatal("文件应已被删除但还能读取数据")
}

func TestMinIOStorage_URL(t *testing.T) {
	store := setupMinIO(t)
	ctx := context.Background()

	content := []byte("test")
	result, err := store.Save(ctx, "url-test.txt", "text/plain", bytes.NewReader(content))
	if err != nil {
		t.Fatalf("保存文件失败: %v", err)
	}
	defer store.Delete(ctx, result.Path)

	url := store.URL(ctx, result.Path)
	if url != result.URL {
		t.Errorf("URL() 返回 %s，但 Save 返回的 URL 为 %s", url, result.URL)
	}
	if url == "" {
		t.Error("URL 不应为空")
	}
	t.Logf("URL: %s", url)
}

func TestMinIOStorage_SaveLargeFile(t *testing.T) {
	store := setupMinIO(t)
	ctx := context.Background()

	// 模拟 1MB 文件
	largeContent := bytes.Repeat([]byte("A"), 1024*1024)

	result, err := store.Save(ctx, "large-file.bin", "application/octet-stream", bytes.NewReader(largeContent))
	if err != nil {
		t.Fatalf("保存大文件失败: %v", err)
	}
	defer store.Delete(ctx, result.Path)

	if result.Size != int64(len(largeContent)) {
		t.Errorf("Size 应为 %d, 实际 %d", len(largeContent), result.Size)
	}

	// 验证能读回
	reader, err := store.Get(ctx, result.Path)
	if err != nil {
		t.Fatalf("读取大文件失败: %v", err)
	}
	defer reader.Close()

	readBack, _ := io.ReadAll(reader)
	if len(readBack) != len(largeContent) {
		t.Errorf("读取大文件大小不一致: 原始 %d, 读取 %d", len(largeContent), len(readBack))
	}
	t.Logf("大文件 (1MB) 保存+读取验证成功")
}

func TestMinIOStorage_MultipleFiles(t *testing.T) {
	store := setupMinIO(t)
	ctx := context.Background()

	type testFile struct {
		name     string
		mime     string
		content  []byte
		result   *UploadedFile
	}

	files := []testFile{
		{name: "发票1.jpg", mime: "image/jpeg", content: []byte{0xFF, 0xD8, 0xFF}},
		{name: "发票2.png", mime: "image/png", content: []byte{0x89, 0x50, 0x4E, 0x47}},
		{name: "发票3.pdf", mime: "application/pdf", content: []byte("%PDF-1.4")},
	}

	// 批量上传
	for i := range files {
		result, err := store.Save(ctx, files[i].name, files[i].mime, bytes.NewReader(files[i].content))
		if err != nil {
			t.Fatalf("保存第 %d 个文件失败: %v", i+1, err)
		}
		files[i].result = result
		t.Logf("上传 #%d: %s → %s", i+1, files[i].name, result.Path)
	}

	// 批量验证
	for i := range files {
		reader, err := store.Get(ctx, files[i].result.Path)
		if err != nil {
			t.Errorf("读取第 %d 个文件失败: %v", i+1, err)
			continue
		}
		readBack, _ := io.ReadAll(reader)
		reader.Close()
		if !bytes.Equal(files[i].content, readBack) {
			t.Errorf("第 %d 个文件内容不一致", i+1)
		}
	}

	// 批量清理
	for i := range files {
		store.Delete(ctx, files[i].result.Path)
	}

	t.Logf("批量上传/验证/清理 %d 个文件成功", len(files))
}

func TestMinIOStorage_SaveEmptyFile(t *testing.T) {
	store := setupMinIO(t)
	ctx := context.Background()

	result, err := store.Save(ctx, "empty.txt", "text/plain", bytes.NewReader([]byte{}))
	if err != nil {
		t.Fatalf("保存空文件失败: %v", err)
	}
	defer store.Delete(ctx, result.Path)

	if result.Size != 0 {
		t.Errorf("空文件 Size 应为 0, 实际 %d", result.Size)
	}

	reader, _ := store.Get(ctx, result.Path)
	readBack, _ := io.ReadAll(reader)
	reader.Close()
	if len(readBack) != 0 {
		t.Errorf("空文件读取应为空, 实际 %d bytes", len(readBack))
	}
	t.Log("空文件保存+读取验证成功")
}

func TestMinIOStorage_PathUniqueness(t *testing.T) {
	store := setupMinIO(t)
	ctx := context.Background()

	content := []byte("same content")

	// 两次保存相同文件名
	r1, _ := store.Save(ctx, "same-name.jpg", "image/jpeg", bytes.NewReader(content))
	r2, _ := store.Save(ctx, "same-name.jpg", "image/jpeg", bytes.NewReader(content))

	if r1.Path == r2.Path {
		t.Errorf("两次保存相同文件名应生成不同路径:\n  r1: %s\n  r2: %s", r1.Path, r2.Path)
	}
	t.Logf("路径唯一性验证: r1=%s, r2=%s", r1.Path, r2.Path)

	store.Delete(ctx, r1.Path)
	store.Delete(ctx, r2.Path)
}
