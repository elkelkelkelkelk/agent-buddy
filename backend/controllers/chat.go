package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

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

type StreamChunk struct {
	Content   string `json:"content,omitempty"`
	Reasoning string `json:"reasoning,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Stream handles SSE streaming with DeepSeek / OpenAI compatible API
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

	var chatMsgs []openai.ChatCompletionMessage
	for _, m := range req.Messages {
		chatMsgs = append(chatMsgs, openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	streamReq := openai.ChatCompletionRequest{
		Model:    model,
		Messages: chatMsgs,
		Stream:   true,
	}

	stream, err := client.CreateChatCompletionStream(c.Request.Context(), streamReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "调用大模型失败: " + err.Error()})
		return
	}
	defer stream.Close()

	// 设置 SSE 头
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	c.Stream(func(w io.Writer) bool {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			fmt.Fprintf(w, "data: [DONE]\n\n")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return false
		}
		if err != nil {
			errBytes, _ := json.Marshal(StreamChunk{Error: err.Error()})
			fmt.Fprintf(w, "data: %s\n\n", errBytes)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return false
		}

		if len(response.Choices) > 0 {
			delta := response.Choices[0].Delta
			if delta.Content != "" || delta.ReasoningContent != "" {
				chunkBytes, _ := json.Marshal(StreamChunk{
					Content:   delta.Content,
					Reasoning: delta.ReasoningContent,
				})
				fmt.Fprintf(w, "data: %s\n\n", chunkBytes)
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
				return true
			}
		}
		return true
	})
}
