package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Massad/gin-boilerplate/tools"
	"github.com/gin-gonic/gin"
	openai "github.com/sashabaranov/go-openai"
)

type ChatController struct{}

type ChatMessage struct {
	Role    string `json:"role" binding:"required"`
	Content string `json:"content" binding:"required"`
}

type ChatStreamRequest struct {
	Messages []ChatMessage `json:"messages" binding:"required"`
}

// 扩展后的分片结构体，支持通知前端工具状态
type StreamChunk struct {
	Content    string `json:"content,omitempty"`     // 最终回答文本
	Reasoning  string `json:"reasoning,omitempty"`   // 深度思考推理
	ToolName   string `json:"tool_name,omitempty"`   // 工具名称
	ToolArgs   string `json:"tool_args,omitempty"`   // 工具入参
	ToolResult string `json:"tool_result,omitempty"` // 工具执行结果
	ToolStatus string `json:"tool_status,omitempty"` // calling (调用中) / done (完成)
	Error      string `json:"error,omitempty"`
}

// Stream handles SSE streaming with DeepSeek / OpenAI compatible API + Tool Calling
func (ctrl *ChatController) Stream(c *gin.Context) {
	var req ChatStreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	apiKey := strings.Trim(strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY")), "\"")
	if apiKey == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "请先在 backend/.env 中配置 DEEPSEEK_API_KEY"})
		return
	}

	baseURL := strings.Trim(strings.TrimSpace(os.Getenv("DEEPSEEK_BASE_URL")), "\"")
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}

	model := strings.Trim(strings.TrimSpace(os.Getenv("DEEPSEEK_MODEL")), "\"")
	if model == "" {
		model = "deepseek-v4.1-flash"
	}

	config := openai.DefaultConfig(apiKey)
	config.BaseURL = baseURL
	client := openai.NewClientWithConfig(config)

	// 组装历史消息
	var chatMsgs []openai.ChatCompletionMessage
	for _, m := range req.Messages {
		chatMsgs = append(chatMsgs, openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	// 注入工具定义
	streamReq := openai.ChatCompletionRequest{
		Model:    model,
		Messages: chatMsgs,
		Stream:   true,
		Tools:    tools.GetToolDefinitions(), // 👈 关键点 1：声明工具
	}

	// 设置 SSE 响应头
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	// 发起第 1 轮大模型请求
	stream, err := client.CreateChatCompletionStream(c.Request.Context(), streamReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "调用大模型失败: " + err.Error()})
		return
	}
	defer stream.Close()

	// 收集工具调用碎片的缓存变量
	var toolCallID string
	var toolName string
	var toolArguments strings.Builder
	hasToolCall := false

	// Helper: 向前端 SSE 发送一个 JSON chunk
	sendChunk := func(chunk StreamChunk) {
		bytes, _ := json.Marshal(chunk)
		fmt.Fprintf(c.Writer, "data: %s\n\n", bytes)
		c.Writer.Flush()
	}

	// 循环读取第 1 轮流式数据
	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			sendChunk(StreamChunk{Error: err.Error()})
			return
		}

		if len(response.Choices) > 0 {
			choice := response.Choices[0]
			delta := choice.Delta

			// 如果模型吐出普通思考或正文，直接推给前端
			if delta.Content != "" || delta.ReasoningContent != "" {
				sendChunk(StreamChunk{
					Content:   delta.Content,
					Reasoning: delta.ReasoningContent,
				})
			}

			// 如果模型吐出工具调用的增量块（注意：流式中参数是逐字到达的）
			if len(delta.ToolCalls) > 0 {
				hasToolCall = true
				tc := delta.ToolCalls[0]
				if tc.ID != "" {
					toolCallID = tc.ID
				}
				if tc.Function.Name != "" {
					toolName = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					toolArguments.WriteString(tc.Function.Arguments)
				}
			}
		}
	}

	// 如果大模型本轮并未决定调工具，直接结束
	if !hasToolCall {
		fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
		c.Writer.Flush()
		return
	}

	// ==================== 核心：触发工具调用与第 2 轮握手 ====================
	fullArgs := toolArguments.String()

	// 1. 先通过 SSE 通知前端：模型已下达指令，正在执行该工具
	sendChunk(StreamChunk{
		ToolName:   toolName,
		ToolArgs:   fullArgs,
		ToolStatus: "calling",
	})

	// 2. 本地代为执行真实函数
	toolResult := tools.ExecuteTool(toolName, fullArgs)

	// 3. 通知前端：工具执行完成，并展示执行结果
	sendChunk(StreamChunk{
		ToolName:   toolName,
		ToolResult: toolResult,
		ToolStatus: "done",
	})

	// 4. 组装二次请求的完整上下文（包含 assistant tool_calls 和 tool 执行结果）
	chatMsgs = append(chatMsgs, openai.ChatCompletionMessage{
		Role: openai.ChatMessageRoleAssistant,
		ToolCalls: []openai.ToolCall{
			{
				ID:   toolCallID,
				Type: openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      toolName,
					Arguments: fullArgs,
				},
			},
		},
	})
	chatMsgs = append(chatMsgs, openai.ChatCompletionMessage{
		Role:       openai.ChatMessageRoleTool,
		ToolCallID: toolCallID,
		Name:       toolName,
		Content:    toolResult,
	})

	// 5. 发起第 2 轮请求，让大模型阅读工具结果并输出最终人话
	secondReq := openai.ChatCompletionRequest{
		Model:    model,
		Messages: chatMsgs,
		Stream:   true,
	}

	secondStream, err := client.CreateChatCompletionStream(c.Request.Context(), secondReq)
	if err != nil {
		sendChunk(StreamChunk{Error: "第 2 轮大模型调用失败: " + err.Error()})
		return
	}
	defer secondStream.Close()

	// 6. 流式将大模型最终整理的文字推给前端
	for {
		resp, err := secondStream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			sendChunk(StreamChunk{Error: err.Error()})
			return
		}

		if len(resp.Choices) > 0 {
			delta := resp.Choices[0].Delta
			if delta.Content != "" || delta.ReasoningContent != "" {
				sendChunk(StreamChunk{
					Content:   delta.Content,
					Reasoning: delta.ReasoningContent,
				})
			}
		}
	}

	// 发送终结标识符
	fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
	c.Writer.Flush()
}
