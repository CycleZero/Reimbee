package vectorstore

import (
	"context"
	"fmt"
	"time"

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
	"github.com/amikos-tech/chroma-go/pkg/embeddings"

	"github.com/CycleZero/Reimbee/log"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// ChromaStore Chroma 向量数据库实现
type ChromaStore struct {
	client         chroma.Client     // Chroma HTTP 客户端
	collection     chroma.Collection // 当前操作的集合句柄
	collectionName string            // 集合名称
	dim            int               // 向量维度
	logger         *log.Logger       // 日志器
}

// NewChromaStore 创建 Chroma 向量库实例
func NewChromaStore(vc *viper.Viper, dim int, logger *log.Logger) (*ChromaStore, error) {
	// 读取配置，设置默认值
	endpoint := vc.GetString("vector_store.chroma.endpoint")
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}
	collectionName := vc.GetString("vector_store.chroma.collection")
	if collectionName == "" {
		collectionName = "reimbee_policies"
	}
	timeoutStr := vc.GetString("vector_store.chroma.timeout")
	if timeoutStr == "" {
		timeoutStr = "30s"
	}

	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		logger.Error("解析Chroma超时配置失败", zap.String("值", timeoutStr), zap.Error(err))
		return nil, fmt.Errorf("解析Chroma超时配置失败：%w", err)
	}

	logger.Info("正在创建Chroma向量库连接",
		zap.String("端点", endpoint),
		zap.String("集合名", collectionName),
		zap.Int("维度", dim),
		zap.Duration("超时", timeout))

	// 创建 Chroma HTTP 客户端
	client, err := chroma.NewHTTPClient(
		chroma.WithBaseURL(endpoint),
		chroma.WithTimeout(timeout),
	)
	if err != nil {
		logger.Error("创建Chroma HTTP客户端失败", zap.String("端点", endpoint), zap.Error(err))
		return nil, fmt.Errorf("创建Chroma HTTP客户端失败：%w", err)
	}

	// 创建或获取集合，使用余弦相似度空间
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	collection, err := client.GetOrCreateCollection(ctx, collectionName,
		chroma.WithHNSWSpaceCreate("cosine"),
		chroma.WithDisableEFConfigStorage(), // 禁用默认嵌入函数，使用者自行提供向量
	)
	if err != nil {
		logger.Error("获取/创建Chroma集合失败",
			zap.String("端点", endpoint),
			zap.String("集合名", collectionName),
			zap.Error(err))
		return nil, fmt.Errorf("连接Chroma服务失败（端点: %s，集合: %s）：%w", endpoint, collectionName, err)
	}

	logger.Info("Chroma向量库连接成功",
		zap.String("集合名", collectionName),
		zap.String("集合ID", collection.ID()))

	return &ChromaStore{
		client:         client,
		collection:     collection,
		collectionName: collectionName,
		dim:            dim,
		logger:         logger,
	}, nil
}

// Name 返回向量库名称
func (s *ChromaStore) Name() string { return "chroma" }

// HealthCheck 健康检查：通过 Chroma 心跳接口判断服务是否可用
func (s *ChromaStore) HealthCheck(ctx context.Context) error {
	err := s.client.Heartbeat(ctx)
	if err != nil {
		s.logger.Error("Chroma健康检查失败", zap.Error(err))
		return fmt.Errorf("Chroma服务不可用：%w", err)
	}
	s.logger.Debug("Chroma健康检查通过")
	return nil
}

// Store 批量存储向量记录
func (s *ChromaStore) Store(ctx context.Context, vectors []Vector) error {
	if len(vectors) == 0 {
		return nil
	}

	n := len(vectors)

	// 将 []Vector 转换为 Chroma API 所需的各列数据
	ids := make([]chroma.DocumentID, n)
	embList := make([]embeddings.Embedding, n)
	texts := make([]string, n)
	metadatas := make([]chroma.DocumentMetadata, n)

	for i, v := range vectors {
		// 维度校验
		if len(v.Embedding) != s.dim {
			return fmt.Errorf("向量维度不匹配：期望 %d，实际 %d（ID: %s）", s.dim, len(v.Embedding), v.ID)
		}

		ids[i] = chroma.DocumentID(v.ID)
		embList[i] = embeddings.NewEmbeddingFromFloat64(v.Embedding)
		texts[i] = v.Content

		// 将 map[string]string 转为 DocumentMetadata
		metaMap := make(map[string]interface{}, len(v.Metadata))
		for mk, mv := range v.Metadata {
			metaMap[mk] = mv
		}
		dm, err := chroma.NewDocumentMetadataFromMap(metaMap)
		if err != nil {
			return fmt.Errorf("转换元数据失败（ID: %s）：%w", v.ID, err)
		}
		metadatas[i] = dm
	}

	err := s.collection.Add(ctx,
		chroma.WithIDs(ids...),
		chroma.WithEmbeddings(embList...),
		chroma.WithTexts(texts...),
		chroma.WithMetadatas(metadatas...),
	)
	if err != nil {
		s.logger.Error("存储向量到Chroma失败", zap.Int("数量", n), zap.Error(err))
		return fmt.Errorf("存储向量到Chroma失败：%w", err)
	}

	s.logger.Debug("向量已存储到Chroma",
		zap.Int("新增数量", n),
		zap.String("集合名", s.collectionName))
	return nil
}

// Search 根据查询向量进行相似度搜索，返回 topK 个最相似的结果
func (s *ChromaStore) Search(ctx context.Context, query []float64, topK int, filters map[string]string) ([]SearchResult, error) {
	if len(query) != s.dim {
		return nil, fmt.Errorf("查询向量维度不匹配：期望 %d，实际 %d", s.dim, len(query))
	}

	if topK <= 0 {
		return []SearchResult{}, nil
	}

	// 将 float64 查询向量转为 Chroma Embedding
	queryEmb := embeddings.NewEmbeddingFromFloat64(query)

	// 构建查询选项
	queryOpts := []chroma.CollectionQueryOption{
		chroma.WithQueryEmbeddings(queryEmb),
		chroma.WithNResults(topK),
		chroma.WithInclude(chroma.IncludeDocuments, chroma.IncludeDistances, chroma.IncludeMetadatas),
	}

	// 构建元数据过滤条件（精确匹配键值对）
	if len(filters) > 0 {
		var whereClauses []chroma.WhereClause
		for k, v := range filters {
			whereClauses = append(whereClauses, chroma.EqString(k, v))
		}
		if len(whereClauses) == 1 {
			queryOpts = append(queryOpts, chroma.WithWhere(whereClauses[0]))
		} else {
			queryOpts = append(queryOpts, chroma.WithWhere(chroma.And(whereClauses...)))
		}
	}

	qr, err := s.collection.Query(ctx, queryOpts...)
	if err != nil {
		s.logger.Error("Chroma相似度搜索失败", zap.Error(err))
		return nil, fmt.Errorf("Chroma相似度搜索失败：%w", err)
	}

	// 单次查询，取第一组结果
	idGroups := qr.GetIDGroups()
	docGroups := qr.GetDocumentsGroups()
	distGroups := qr.GetDistancesGroups()
	metaGroups := qr.GetMetadatasGroups()

	if len(idGroups) == 0 || len(idGroups[0]) == 0 {
		return []SearchResult{}, nil
	}

	ids := idGroups[0]
	resultCount := len(ids)

	// 确保各组存在且长度一致
	var docs chroma.Documents
	if len(docGroups) > 0 {
		docs = docGroups[0]
	}
	var dists []float64
	if len(distGroups) > 0 {
		dists = make([]float64, len(distGroups[0]))
		for i, d := range distGroups[0] {
			dists[i] = float64(d)
		}
	}
	var metas chroma.DocumentMetadatas
	if len(metaGroups) > 0 {
		metas = metaGroups[0]
	}

	results := make([]SearchResult, resultCount)
	for i := 0; i < resultCount; i++ {
		result := SearchResult{
			ID: string(ids[i]),
		}

		// 提取文本内容
		if i < len(docs) && docs[i] != nil {
			result.Content = docs[i].ContentString()
		}

		// 将 Chroma 距离转为相似度分数（余弦距离 -> 相似度：1 - distance）
		if i < len(dists) {
			result.Score = 1.0 - dists[i]
		}

		// 提取元数据（DocumentMetadata 接口不含 Keys()，需断言到具体类型）
		if i < len(metas) && metas[i] != nil {
			result.Metadata = make(map[string]string)
			if impl, ok := metas[i].(*chroma.DocumentMetadataImpl); ok {
				for _, key := range impl.Keys() {
					if str, ok := impl.GetString(key); ok {
						result.Metadata[key] = str
					}
				}
			}
		}

		results[i] = result
	}

	s.logger.Debug("Chroma相似度搜索完成",
		zap.Int("返回数量", resultCount))
	return results, nil
}

// Delete 根据 ID 列表批量删除向量记录，不存在的 ID 静默忽略
func (s *ChromaStore) Delete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	docIDs := make([]chroma.DocumentID, len(ids))
	for i, id := range ids {
		docIDs[i] = chroma.DocumentID(id)
	}

	err := s.collection.Delete(ctx, chroma.WithIDs(docIDs...))
	if err != nil {
		s.logger.Error("从Chroma删除向量失败", zap.Int("数量", len(ids)), zap.Error(err))
		return fmt.Errorf("从Chroma删除向量失败：%w", err)
	}

	s.logger.Debug("向量已从Chroma删除",
		zap.Int("删除数量", len(ids)),
		zap.String("集合名", s.collectionName))
	return nil
}

// Clear 清空当前集合中的所有向量记录（删除并重建集合）
func (s *ChromaStore) Clear(ctx context.Context) error {
	// 删除现有集合
	err := s.client.DeleteCollection(ctx, s.collectionName)
	if err != nil {
		s.logger.Error("删除Chroma集合失败",
			zap.String("集合名", s.collectionName),
			zap.Error(err))
		return fmt.Errorf("清空Chroma集合失败：%w", err)
	}

	// 重新创建集合
	newCol, err := s.client.CreateCollection(ctx, s.collectionName,
		chroma.WithHNSWSpaceCreate("cosine"),
		chroma.WithDisableEFConfigStorage(),
	)
	if err != nil {
		s.logger.Error("重建Chroma集合失败",
			zap.String("集合名", s.collectionName),
			zap.Error(err))
		return fmt.Errorf("重建Chroma集合失败：%w", err)
	}

	s.collection = newCol
	s.logger.Info("Chroma集合已清空并重建",
		zap.String("集合名", s.collectionName))
	return nil
}

// Count 返回当前集合中的向量记录总数
func (s *ChromaStore) Count(ctx context.Context) (int64, error) {
	count, err := s.collection.Count(ctx)
	if err != nil {
		s.logger.Error("获取Chroma集合数量失败",
			zap.String("集合名", s.collectionName),
			zap.Error(err))
		return 0, fmt.Errorf("获取Chroma集合数量失败：%w", err)
	}

	s.logger.Debug("Chroma集合数量统计",
		zap.Int64("数量", int64(count)),
		zap.String("集合名", s.collectionName))
	return int64(count), nil
}
