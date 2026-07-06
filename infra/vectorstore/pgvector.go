package vectorstore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/CycleZero/Reimbee/log"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	pgxvec "github.com/pgvector/pgvector-go/pgx"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// PgvectorStore 基于 PostgreSQL pgvector 扩展的向量数据库实现
// 底层使用 pgxpool 连接池，通过 pgvector-go 库操作向量数据，
// 支持余弦相似度搜索和 JSONB 元数据过滤。
type PgvectorStore struct {
	pool      *pgxpool.Pool // pgx 连接池
	tableName string        // 向量表名
	dim       int           // 向量维度，用于建表时的 vector(N) 声明
	logger    *log.Logger   // 日志记录器
}

// NewPgvectorStore 创建 pgvector 向量库实例
//
// 从 Viper 配置中读取 DSN 和表名：
//   - vector_store.pgvector.dsn：PostgreSQL 连接串（默认 "postgres://localhost:5432/reimbee?sslmode=disable"）
//   - vector_store.pgvector.table：向量表名（默认 "reimbee_policies"）
//
// 连接成功后自动执行初始化：
//  1. 注册 pgvector 数据类型到 pgx 连接池
//  2. CREATE EXTENSION IF NOT EXISTS vector
//  3. CREATE TABLE IF NOT EXISTS（id, content, embedding, metadata, created_at）
//  4. CREATE INDEX IF NOT EXISTS（HNSW 余弦相似度索引）
func NewPgvectorStore(vc *viper.Viper, dim int, logger *log.Logger) (*PgvectorStore, error) {
	// 读取 DSN 配置，提供默认值
	dsn := vc.GetString("vector_store.pgvector.dsn")
	if dsn == "" {
		dsn = "postgres://localhost:5432/reimbee?sslmode=disable"
	}

	// 读取表名配置，提供默认值
	tableName := vc.GetString("vector_store.pgvector.table")
	if tableName == "" {
		tableName = "reimbee_policies"
	}

	logger.Info("正在连接 pgvector 向量库",
		zap.String("表名", tableName),
		zap.Int("维度", dim))

	ctx := context.Background()

	// 解析连接池配置
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("pgvector DSN 解析失败: %w", err)
	}

	// 注册 pgvector 类型：每个新建连接都需要注册 vector 类型到 pgx 类型系统
	poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvec.RegisterTypes(ctx, conn)
	}

	// 创建连接池
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("pgvector 连接池创建失败: %w", err)
	}

	// 初始化数据库结构（扩展、表、索引）
	if err := initPgvectorSchema(ctx, pool, tableName, dim); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgvector 数据库结构初始化失败: %w", err)
	}

	logger.Info("pgvector 向量库连接成功",
		zap.String("表名", tableName),
		zap.Int("维度", dim))

	return &PgvectorStore{
		pool:      pool,
		tableName: tableName,
		dim:       dim,
		logger:    logger,
	}, nil
}

// initPgvectorSchema 初始化 pgvector 数据库结构
//
// 依次执行：
//  1. 启用 vector 扩展
//  2. 创建向量表（如不存在）
//  3. 创建 HNSW 余弦相似度索引（如不存在）
func initPgvectorSchema(ctx context.Context, pool *pgxpool.Pool, tableName string, dim int) error {
	// 创建 pgvector 扩展（如不存在）
	_, err := pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
	if err != nil {
		return fmt.Errorf("创建 vector 扩展失败: %w", err)
	}

	// 创建向量表（如不存在）
	// embedding 列类型为 vector(N)，N 为向量维度
	createTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id          VARCHAR(128) PRIMARY KEY,
			content     TEXT NOT NULL,
			embedding   vector(%d),
			metadata    JSONB DEFAULT '{}',
			created_at  TIMESTAMP DEFAULT NOW()
		)`, tableName, dim)
	_, err = pool.Exec(ctx, createTableSQL)
	if err != nil {
		return fmt.Errorf("创建向量表 %s 失败: %w", tableName, err)
	}

	// 创建 HNSW 余弦相似度索引（如不存在）
	// vector_cosine_ops 使用余弦距离运算符（<=>）
	createIndexSQL := fmt.Sprintf(
		"CREATE INDEX IF NOT EXISTS %s_embedding_idx ON %s USING hnsw (embedding vector_cosine_ops)",
		tableName, tableName)
	_, err = pool.Exec(ctx, createIndexSQL)
	if err != nil {
		return fmt.Errorf("创建向量索引失败: %w", err)
	}

	return nil
}

// Name 返回向量库名称，用于日志和诊断
func (s *PgvectorStore) Name() string { return "pgvector" }

// HealthCheck 健康检查，通过执行 SELECT 1 判断数据库连接是否正常
func (s *PgvectorStore) HealthCheck(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// Store 批量存储向量记录，使用 INSERT ... ON CONFLICT DO UPDATE 实现 upsert 语义
func (s *PgvectorStore) Store(ctx context.Context, vectors []Vector) error {
	if len(vectors) == 0 {
		return nil
	}

	upsertSQL := fmt.Sprintf(`
		INSERT INTO %s (id, content, embedding, metadata)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET
			embedding = EXCLUDED.embedding,
			content   = EXCLUDED.content,
			metadata  = EXCLUDED.metadata`, s.tableName)

	for i, v := range vectors {
		if len(v.Embedding) != s.dim {
			return fmt.Errorf("向量维度不匹配：期望 %d，实际 %d（ID: %s）", s.dim, len(v.Embedding), v.ID)
		}

		metaJSON, err := marshalMetadata(v.Metadata)
		if err != nil {
			return fmt.Errorf("序列化 metadata 失败（ID: %s）: %w", v.ID, err)
		}

		_, err = s.pool.Exec(ctx, upsertSQL,
			v.ID, v.Content, pgvector.NewVector(float64ToFloat32(v.Embedding)), metaJSON)
		if err != nil {
			return fmt.Errorf("存储向量失败（第 %d 条，ID: %s）: %w", i+1, v.ID, err)
		}
	}

	s.logger.Debug("向量已存储到 pgvector",
		zap.Int("存储数量", len(vectors)),
		zap.String("表名", s.tableName))
	return nil
}

// Search 根据查询向量进行余弦相似度搜索
//
// 使用 pgvector 的余弦距离运算符 <=>，通过 1 - (embedding <=> $1) 将距离转换为相似度分数。
// filters 参数为可选的元数据过滤条件，使用 JSONB @> 运算符进行包含匹配。
// 返回 topK 个最相似的结果，按相似度降序排列。
func (s *PgvectorStore) Search(ctx context.Context, query []float64, topK int, filters map[string]string) ([]SearchResult, error) {
	if len(query) != s.dim {
		return nil, fmt.Errorf("查询向量维度不匹配：期望 %d，实际 %d", s.dim, len(query))
	}

	vec := float64ToFloat32(query)
	queryVec := pgvector.NewVector(vec)

	// 构建带过滤条件的 SQL
	var sql string
	var args []any

	if len(filters) > 0 {
		// 将 filters map 转为 JSONB 字符串，使用 @> 运算符进行包含匹配
		filterJSON, err := json.Marshal(filters)
		if err != nil {
			return nil, fmt.Errorf("序列化过滤条件失败: %w", err)
		}
		sql = fmt.Sprintf(`
			SELECT id, content, metadata, 1 - (embedding <=> $1) AS similarity
			FROM %s
			WHERE metadata @> $3::jsonb
			ORDER BY embedding <=> $1
			LIMIT $2`, s.tableName)
		args = []any{queryVec, topK, string(filterJSON)}
	} else {
		sql = fmt.Sprintf(`
			SELECT id, content, metadata, 1 - (embedding <=> $1) AS similarity
			FROM %s
			ORDER BY embedding <=> $1
			LIMIT $2`, s.tableName)
		args = []any{queryVec, topK}
	}

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("pgvector 相似度搜索失败: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var metaBytes []byte
		if err := rows.Scan(&r.ID, &r.Content, &metaBytes, &r.Score); err != nil {
			return nil, fmt.Errorf("扫描搜索结果失败: %w", err)
		}
		// 反序列化 metadata JSONB
		if err := unmarshalMetadata(metaBytes, &r.Metadata); err != nil {
			return nil, fmt.Errorf("解析搜索结果 metadata 失败（ID: %s）: %w", r.ID, err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历搜索结果失败: %w", err)
	}

	s.logger.Debug("pgvector 相似度搜索完成",
		zap.Int("返回数量", len(results)),
		zap.String("表名", s.tableName))
	return results, nil
}

// Delete 根据 ID 列表批量删除向量记录，不存在的 ID 静默忽略
func (s *PgvectorStore) Delete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	sql := fmt.Sprintf("DELETE FROM %s WHERE id = ANY($1)", s.tableName)
	tag, err := s.pool.Exec(ctx, sql, ids)
	if err != nil {
		return fmt.Errorf("pgvector 删除向量失败: %w", err)
	}

	s.logger.Debug("向量已从 pgvector 删除",
		zap.Int64("删除数量", tag.RowsAffected()),
		zap.String("表名", s.tableName))
	return nil
}

// Clear 清空向量表中的所有记录
func (s *PgvectorStore) Clear(ctx context.Context) error {
	sql := fmt.Sprintf("TRUNCATE TABLE %s", s.tableName)
	_, err := s.pool.Exec(ctx, sql)
	if err != nil {
		return fmt.Errorf("pgvector 清空向量表失败: %w", err)
	}

	s.logger.Debug("pgvector 向量表已清空",
		zap.String("表名", s.tableName))
	return nil
}

// Count 返回当前向量表中的记录总数
func (s *PgvectorStore) Count(ctx context.Context) (int64, error) {
	sql := fmt.Sprintf("SELECT COUNT(*) FROM %s", s.tableName)
	var count int64
	err := s.pool.QueryRow(ctx, sql).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("pgvector 统计向量数量失败: %w", err)
	}
	return count, nil
}

// =============================================================================
// 辅助函数
// =============================================================================

// float64ToFloat32 将 float64 切片转换为 float32 切片
// pgvector.NewVector 需要 float32 类型的输入
func float64ToFloat32(v []float64) []float32 {
	vec := make([]float32, len(v))
	for i, val := range v {
		vec[i] = float32(val)
	}
	return vec
}

// marshalMetadata 将 metadata map 序列化为 JSON 字节
func marshalMetadata(metadata map[string]string) ([]byte, error) {
	if metadata == nil {
		metadata = make(map[string]string)
	}
	return json.Marshal(metadata)
}

// unmarshalMetadata 将 JSON 字节反序列化为 metadata map
func unmarshalMetadata(data []byte, target *map[string]string) error {
	if data == nil {
		*target = make(map[string]string)
		return nil
	}
	return json.Unmarshal(data, target)
}


