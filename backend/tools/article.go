package tools

import (
	"encoding/json"
	"fmt"
)

// 1. 删除文章的入参结构
type DeleteArticleParams struct {
	ArticleID string `json:"article_id" jsonschema:"description=要删除的文章ID，例如: 88, 102"`
}

// 2. 本地真实业务删除函数
func DeleteArticle(arguments string) string {
	var params DeleteArticleParams
	if err := json.Unmarshal([]byte(arguments), &params); err != nil {
		return fmt.Sprintf(`{"error": "参数解析失败: %v"}`, err)
	}

	// 模拟执行数据库删除
	return fmt.Sprintf(`{"status": "success", "article_id": "%s", "message": "文章已从数据库中永久物理删除！"}`, params.ArticleID)
}

// 3. 高危工具白名单判断：只要是这里面的工具，大模型都不能直接调用，必须走审批！
func IsDangerousTool(toolName string) bool {
	dangerousTools := map[string]bool{
		"delete_article": true, // 标记为高危工具
	}
	return dangerousTools[toolName]
}
