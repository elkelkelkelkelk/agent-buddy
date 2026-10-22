package tools

import (
	"encoding/json"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

// 1. 定义该工具入参的 Go 结构体
type WeatherParams struct {
	Location string `json:"location" jsonschema:"description=要查询天气的城市名称，如：杭州、北京"`
}

// 2. 本地真实业务函数（这里模拟调用三方天气 API，实际中也可接高德天气 API 或查数据库）
func GetWeather(arguments string) string {
	var params WeatherParams
	if err := json.Unmarshal([]byte(arguments), &params); err != nil {
		return fmt.Sprintf(`{"error": "参数解析失败: %v"}`, err)
	}

	// 模拟返回结构化数据
	return fmt.Sprintf(`{"location": "%s", "weather": "晴朗", "temperature": "26℃", "wind": "东南风 2 级", "tips": "气温适宜，推荐外出！"}`, params.Location)
}

// 3. 构建提供给大模型的 OpenAI Tools 声明清单
func GetToolDefinitions() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "get_weather",
				Description: "查询指定城市的当前实时天气情况、气温以及出行穿衣建议",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"location": {
							Type:        jsonschema.String,
							Description: "城市名称，例如：杭州、上海、北京",
						},
					},
					Required: []string{"location"},
				},
			},
		},
	}
}

// 4. 工具路由器：根据大模型返回的名字分发执行
func ExecuteTool(name string, args string) string {
	switch name {
	case "get_weather":
		return GetWeather(args)
	default:
		return fmt.Sprintf(`{"error": "未知工具: %s"}`, name)
	}
}
