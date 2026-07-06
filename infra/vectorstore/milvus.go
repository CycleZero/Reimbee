package vectorstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/CycleZero/Reimbee/log"
	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// 常量定义
const (
	defaultMilvusEndpoint   = "localhost:19530"
	defaultMilvusCollection = "reimbee_policies"
	defaultMilvusTimeout    = "30s"
	idFieldName             = "id"
	contentFieldName        = "content"
	embeddingFieldName      = "embedding"
	metadataFieldName       = "metadata"
)

// MilvusStore Milvus 向量数据库实现
type MilvusStore struct {
	client         *milvusclient.Client
	collectionName string
	dim            int
	logger         *log.Logger
}

// NewMilvusStore 创建 Milvus 向量库实例
// 读取配置、建立连接、创建/获取集合，并对向量字段建立自动索引。
// 若 Milvus 服务不可达，返回描述性错误而非 panic。
func NewMilvusStore(vc *viper.Viper, dim int, logger *log.Logger) (*MilvusStore, error) {
	// 读取配置
	endpoint := vc.GetString("vector_store.milvus.endpoint")
	if endpoint == "" {
		endpoint = defaultMilvusEndpoint
	}
	collectionName := vc.GetString("vector_store.milvus.collection")
	if collectionName == "" {
		collectionName = defaultMilvusCollection
	}
	timeoutStr := vc.GetString("vector_store.milvus.timeout")
	if timeoutStr == "" {
		timeoutStr = defaultMilvusTimeout
	}
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		logger.Warn("Milvus超时配置解析失败，使用默认值",
			zap.String("配置值", timeoutStr),
			zap.String("默认值", defaultMilvusTimeout),
			zap.Error(err))
		timeout, _ = time.ParseDuration(defaultMilvusTimeout)
	}

	logger.Info("正在连接Milvus向量库",
		zap.String("endpoint", endpoint),
		zap.String("集合名称", collectionName),
		zap.Int("向量维度", dim),
		zap.Duration("超时", timeout))

	// 创建 Milvus 客户端
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cli, err := milvusclient.New(ctx, &milvusclient.ClientConfig{
		Address: endpoint,
	})
	if err != nil {
		return nil, fmt.Errorf("连接Milvus失败 (endpoint=%s): %w", endpoint, err)
	}

	store := &MilvusStore{
		client:         cli,
		collectionName: collectionName,
		dim:            dim,
		logger:         logger,
	}

	// 创建或获取集合
	if err := store.ensureCollection(ctx, timeout); err != nil {
		_ = cli.Close(context.Background())
		return nil, fmt.Errorf("初始化Milvus集合失败: %w", err)
	}

	logger.Info("Milvus向量库初始化成功",
		zap.String("集合名称", collectionName),
		zap.Int("向量维度", dim))
	return store, nil
}

// ensureCollection 确保集合存在：存在则跳过，不存在则创建并建立索引
func (s *MilvusStore) ensureCollection(ctx context.Context, timeout time.Duration) error {
	// 检查集合是否已存在
	hasCollCtx, hasCollCancel := context.WithTimeout(context.Background(), timeout)
	defer hasCollCancel()

	has, err := s.client.HasCollection(hasCollCtx, milvusclient.NewHasCollectionOption(s.collectionName))
	if err != nil {
		s.logger.Warn("检查Milvus集合存在性失败，将尝试创建",
			zap.String("集合名称", s.collectionName),
			zap.Error(err))
		// 不直接返回错误，尝试创建（幂等操作）
	}

	if has {
		s.logger.Debug("Milvus集合已存在，复用现有集合",
			zap.String("集合名称", s.collectionName))
		return nil
	}

	// 构建自定义 schema
	schema := entity.NewSchema().
		WithField(
			entity.NewField().
				WithName(idFieldName).
				WithDataType(entity.FieldTypeVarChar).
				WithMaxLength(256).
				WithIsPrimaryKey(true),
		).
		WithField(
			entity.NewField().
				WithName(contentFieldName).
				WithDataType(entity.FieldTypeVarChar).
				WithMaxLength(65535),
		).
		WithField(
			entity.NewField().
				WithName(embeddingFieldName).
				WithDataType(entity.FieldTypeFloatVector).
				WithDim(int64(s.dim)),
		).
		WithField(
			entity.NewField().
				WithName(metadataFieldName).
				WithDataType(entity.FieldTypeJSON),
		)

	createOpt := milvusclient.NewCreateCollectionOption(s.collectionName, schema).
		WithConsistencyLevel(entity.ClBounded).
		WithIndexOptions(
			milvusclient.NewCreateIndexOption(s.collectionName, embeddingFieldName, index.NewAutoIndex(entity.COSINE)).
				WithIndexName(embeddingFieldName + "_idx"),
		)

	createCtx, createCancel := context.WithTimeout(context.Background(), timeout)
	defer createCancel()

	if err := s.client.CreateCollection(createCtx, createOpt); err != nil {
		return fmt.Errorf("创建Milvus集合 %s 失败: %w", s.collectionName, err)
	}

	s.logger.Info("Milvus集合创建成功，正在加载集合",
		zap.String("集合名称", s.collectionName),
		zap.Int("向量维度", s.dim))

	// 加载集合到内存（非 fast 模式需手动加载）
	loadCtx, loadCancel := context.WithTimeout(context.Background(), timeout*2)
	defer loadCancel()

	loadTask, err := s.client.LoadCollection(loadCtx, milvusclient.NewLoadCollectionOption(s.collectionName))
	if err != nil {
		return fmt.Errorf("加载Milvus集合 %s 失败: %w", s.collectionName, err)
	}
	if err := loadTask.Await(loadCtx); err != nil {
		return fmt.Errorf("等待Milvus集合 %s 加载完成失败: %w", s.collectionName, err)
	}

	s.logger.Info("Milvus集合加载完成", zap.String("集合名称", s.collectionName))
	return nil
}

// Name 返回向量库名称
func (s *MilvusStore) Name() string {
	return "milvus"
}

// HealthCheck 健康检查：验证 Milvus 服务连接可用
func (s *MilvusStore) HealthCheck(ctx context.Context) error {
	collections, err := s.client.ListCollections(ctx, milvusclient.NewListCollectionOption())
	if err != nil {
		return fmt.Errorf("Milvus健康检查失败: %w", err)
	}
	s.logger.Debug("Milvus健康检查通过",
		zap.Int("集合数量", len(collections)))
	return nil
}

// Store 批量存储向量记录
// 使用列式插入，分别填充 id、content、embedding 和 metadata 列。
func (s *MilvusStore) Store(ctx context.Context, vectors []Vector) error {
	if len(vectors) == 0 {
		s.logger.Debug("Milvus Store收到空向量列表，跳过插入")
		return nil
	}

	n := len(vectors)
	ids := make([]string, n)
	contents := make([]string, n)
	embeddings := make([][]float32, n)
	metadataJsons := make([][]byte, n)

	for i, v := range vectors {
		// 维度校验
		if len(v.Embedding) != s.dim {
			return fmt.Errorf("向量维度不匹配：期望 %d，实际 %d（ID: %s）", s.dim, len(v.Embedding), v.ID)
		}

		ids[i] = v.ID
		contents[i] = v.Content

		// 将 float64 嵌入向量转换为 Milvus 所需的 float32
		emb := make([]float32, len(v.Embedding))
		for j, f := range v.Embedding {
			emb[j] = float32(f)
		}
		embeddings[i] = emb

		// 序列化元数据为 JSON
		if v.Metadata != nil && len(v.Metadata) > 0 {
			metaBytes, err := json.Marshal(v.Metadata)
			if err != nil {
				return fmt.Errorf("序列化元数据失败（ID: %s）: %w", v.ID, err)
			}
			metadataJsons[i] = metaBytes
		} else {
			metadataJsons[i] = []byte("{}")
		}
	}

	opt := milvusclient.NewColumnBasedInsertOption(s.collectionName).
		WithVarcharColumn(idFieldName, ids).
		WithVarcharColumn(contentFieldName, contents).
		WithFloatVectorColumn(embeddingFieldName, s.dim, embeddings).
		WithColumns(column.NewColumnJSONBytes(metadataFieldName, metadataJsons))

	result, err := s.client.Insert(ctx, opt)
	if err != nil {
		return fmt.Errorf("Milvus批量插入失败（数量: %d）: %w", n, err)
	}

	s.logger.Debug("向量已存储到Milvus",
		zap.Int("新增数量", n),
		zap.Int64("插入行数", result.InsertCount))
	return nil
}

// Search 根据查询向量进行相似度搜索，返回 topK 个最相似的结果
// filters 为可选的元数据过滤条件（键值对精确匹配），传 nil 或空 map 表示不过滤。
func (s *MilvusStore) Search(ctx context.Context, query []float64, topK int, filters map[string]string) ([]SearchResult, error) {
	if len(query) != s.dim {
		return nil, fmt.Errorf("查询向量维度不匹配：期望 %d，实际 %d", s.dim, len(query))
	}

	// 将 float64 查询向量转换为 Milvus FloatVector
	queryVec := make(entity.FloatVector, len(query))
	for i, f := range query {
		queryVec[i] = float32(f)
	}

	// 构建搜索选项
	sr := milvusclient.NewSearchOption(s.collectionName, topK, []entity.Vector{queryVec}).
		WithANNSField(embeddingFieldName).
		WithOutputFields(idFieldName, contentFieldName, metadataFieldName).
		WithConsistencyLevel(entity.ClBounded)

	// 构建过滤表达式
	if len(filters) > 0 {
		expr := buildFilterExpr(filters)
		if expr != "" {
			sr = sr.WithFilter(expr)
			s.logger.Debug("Milvus搜索应用元数据过滤",
				zap.String("过滤表达式", expr))
		}
	}

	resultSets, err := s.client.Search(ctx, sr)
	if err != nil {
		return nil, fmt.Errorf("Milvus搜索失败: %w", err)
	}

	if len(resultSets) == 0 {
		s.logger.Debug("Milvus搜索无结果")
		return []SearchResult{}, nil
	}

	rs := resultSets[0] // 单查询向量只返回一个 ResultSet
	if rs.Err != nil {
		return nil, fmt.Errorf("Milvus搜索结果异常: %w", rs.Err)
	}

	// 转换搜索结果
	results := make([]SearchResult, 0, rs.ResultCount)
	idColumn := rs.IDs
	contentCol := rs.GetColumn(contentFieldName)
	metaCol := rs.GetColumn(metadataFieldName)

	for i := 0; i < rs.ResultCount; i++ {
		id, err := idColumn.Get(i)
		if err != nil {
			s.logger.Warn("Milvus搜索结果ID提取失败", zap.Int("索引", i), zap.Error(err))
			continue
		}

		content, err := contentCol.Get(i)
		if err != nil {
			s.logger.Warn("Milvus搜索结果内容提取失败", zap.Int("索引", i), zap.Error(err))
			content = ""
		}

		var metadata map[string]string
		if metaCol != nil {
			metaVal, err := metaCol.Get(i)
			if err == nil {
				if metaBytes, ok := metaVal.([]byte); ok {
					_ = json.Unmarshal(metaBytes, &metadata)
				} else if metaStr, ok := metaVal.(string); ok {
					_ = json.Unmarshal([]byte(metaStr), &metadata)
				}
			}
		}
		if metadata == nil {
			metadata = make(map[string]string)
		}

		score := float64(rs.Scores[i])

		results = append(results, SearchResult{
			ID:       fmt.Sprintf("%v", id),
			Content:  fmt.Sprintf("%v", content),
			Score:    score,
			Metadata: metadata,
		})
	}

	s.logger.Debug("Milvus搜索完成",
		zap.Int("返回数量", len(results)),
		zap.Int("请求topK", topK))
	return results, nil
}

// buildFilterExpr 根据元数据过滤条件构建 Milvus 布尔表达式
// 格式: metadata["key"] == "value" && metadata["key2"] == "value2"
func buildFilterExpr(filters map[string]string) string {
	if len(filters) == 0 {
		return ""
	}

	expr := ""
	for k, v := range filters {
		if expr != "" {
			expr += " && "
		}
		expr += fmt.Sprintf(`%s["%s"] == "%s"`, metadataFieldName, k, v)
	}
	return expr
}

// Delete 根据 ID 列表批量删除向量记录
// 不存在的 ID 静默忽略（Milvus 本身行为）。
func (s *MilvusStore) Delete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		s.logger.Debug("Milvus Delete收到空ID列表，跳过删除")
		return nil
	}

	delOpt := milvusclient.NewDeleteOption(s.collectionName).
		WithStringIDs(idFieldName, ids)

	result, err := s.client.Delete(ctx, delOpt)
	if err != nil {
		return fmt.Errorf("Milvus删除失败（ID数量: %d）: %w", len(ids), err)
	}

	s.logger.Debug("向量已从Milvus删除",
		zap.Int("请求删除数量", len(ids)),
		zap.Int64("实际删除数量", result.DeleteCount))
	return nil
}

// Clear 清空集合中的所有向量记录
// 通过删除集合并重建实现。
func (s *MilvusStore) Clear(ctx context.Context) error {
	s.logger.Info("正在清空Milvus集合", zap.String("集合名称", s.collectionName))

	// 删除集合
	dropOpt := milvusclient.NewDropCollectionOption(s.collectionName)
	if err := s.client.DropCollection(ctx, dropOpt); err != nil {
		return fmt.Errorf("删除Milvus集合 %s 失败: %w", s.collectionName, err)
	}

	// 重新创建集合
	timeout := 60 * time.Second
	if err := s.ensureCollection(ctx, timeout); err != nil {
		return fmt.Errorf("重建Milvus集合 %s 失败: %w", s.collectionName, err)
	}

	s.logger.Info("Milvus集合已清空并重建", zap.String("集合名称", s.collectionName))
	return nil
}

// Count 返回当前集合中的向量记录总数
func (s *MilvusStore) Count(ctx context.Context) (int64, error) {
	stats, err := s.client.GetCollectionStats(ctx, milvusclient.NewGetCollectionStatsOption(s.collectionName))
	if err != nil {
		return 0, fmt.Errorf("获取Milvus集合统计信息失败: %w", err)
	}

	rowCountStr, ok := stats["row_count"]
	if !ok {
		return 0, fmt.Errorf("Milvus集合统计信息中缺少 row_count 字段")
	}

	count, err := strconv.ParseInt(rowCountStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("解析Milvus行数失败（值: %s）: %w", rowCountStr, err)
	}

	return count, nil
}
