package memory

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
)

// openOrCreateBleveIndex 打开或创建 bleve 索引。
func openOrCreateBleveIndex(indexPath string) (bleve.Index, error) {
	// 确保索引目录的父目录存在
	parentDir := filepath.Dir(indexPath)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return nil, fmt.Errorf("create index parent dir: %w", err)
	}

	// 尝试打开已有索引
	idx, err := bleve.Open(indexPath)
	if err == nil {
		return idx, nil
	}

	// 索引不存在，创建新的
	mapping := buildBleveMapping()
	idx, err = bleve.New(indexPath, mapping)
	if err != nil {
		return nil, fmt.Errorf("create bleve index: %w", err)
	}
	return idx, nil
}

// buildBleveMapping 构建 bleve 索引映射。
func buildBleveMapping() *mapping.IndexMappingImpl {
	return bleve.NewIndexMapping()
}

// indexBleveDocument 将单条记忆文档加入索引。
func indexBleveDocument(idx bleve.Index, entry *MemoryEntry) error {
	doc := entry.ToBleveDocument()
	return idx.Index(entry.Slug(), doc)
}

// searchBleveIndex 在 bleve 索引中搜索，返回匹配的 slug 列表。
// 使用 DisjunctionQuery + MatchQuery 对多个字段进行匹配，
// 避免 QueryStringQuery 对中文处理不佳的问题。
func searchBleveIndex(idx bleve.Index, queryString string, topK int) ([]string, error) {
	// 对 name、description、content 三个字段分别做 MatchQuery
	nameMatch := bleve.NewMatchQuery(queryString)
	nameMatch.SetField("name")

	descMatch := bleve.NewMatchQuery(queryString)
	descMatch.SetField("description")

	contentMatch := bleve.NewMatchQuery(queryString)
	contentMatch.SetField("content")

	orQuery := bleve.NewDisjunctionQuery(nameMatch, descMatch, contentMatch)

	searchRequest := bleve.NewSearchRequest(orQuery)
	searchRequest.Size = topK
	searchRequest.Fields = []string{"name", "description", "type"}

	result, err := idx.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("bleve search: %w", err)
	}

	var slugs []string
	for _, hit := range result.Hits {
		slugs = append(slugs, hit.ID)
	}

	return slugs, nil
}

// searchBleveIndexBySession 在 bleve 索引中搜索指定会话的记忆，返回匹配的 slug 列表。
// 使用 ConjunctionQuery 连接全文搜索条件和 session_id 过滤条件。
func searchBleveIndexBySession(idx bleve.Index, sessionID, queryString string, topK int) ([]string, error) {
	// 对 name、description、content 三个字段分别做 MatchQuery
	nameMatch := bleve.NewMatchQuery(queryString)
	nameMatch.SetField("name")

	descMatch := bleve.NewMatchQuery(queryString)
	descMatch.SetField("description")

	contentMatch := bleve.NewMatchQuery(queryString)
	contentMatch.SetField("content")

	// session_id 精确匹配
	sessionTerm := bleve.NewTermQuery(sessionID)
	sessionTerm.SetField("session_id")

	// AND 连接：全文搜索 OR + session_id 过滤
	textOrQuery := bleve.NewDisjunctionQuery(nameMatch, descMatch, contentMatch)
	andQuery := bleve.NewConjunctionQuery(textOrQuery, sessionTerm)

	searchRequest := bleve.NewSearchRequest(andQuery)
	searchRequest.Size = topK
	searchRequest.Fields = []string{"name", "description", "type"}

	result, err := idx.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("bleve search by session: %w", err)
	}

	var slugs []string
	for _, hit := range result.Hits {
		slugs = append(slugs, hit.ID)
	}

	return slugs, nil
}
