# 阶段三·实战操作手册：手把手实现 HITL 人机协同审批

> **目标**：在本项目中完整跑通一套生产级 **HITL（Human-in-the-Loop 人机协同审批）** 闭环。  
> 当用户对 AI 说：“**帮我把 ID 为 88 的文章删掉**”时：
> 1. AI 决定调用删除工具；
> 2. 后端拦截器识别为**高危写操作**，强行挂起，向前端推送**【高危审批卡片】**；
> 3. 前端界面展示警告边框与两个按钮：**【批准执行】** 和 **【拒绝取消】**；
> 4. 人类点击后，触发二次流式响应，AI 根据人类决策继续完成或取消操作。

---

## 目录
- [一、核心架构与改造清单](#一核心架构与改造清单)
- [二、Step 1：后端创建高危工具与权限分类](#二step-1后端创建高危工具与权限分类)
- [三、Step 2：后端拦截器与审批任务缓存](#二step-2后端拦截器与审批任务缓存)
- [四、Step 3：后端增加审批决策接口（POST /v1/chat/approve）](#四step-3后端增加审批决策接口post-v1chatapprove)
- [五、Step 4：前端 Vue3 审批卡片与按钮交互](#五step-4前端-vue3-审批卡片与按钮交互)
- [六、连调验证与大白话面试复盘](#六连调验证与大白话面试复盘)

---

## 一、核心架构与改造清单

本次实操只需修改/新增 4 处内容，结构非常清晰：

| 序号 | 文件路径 | 动作 | 核心职责 |
| :--- | :--- | :--- | :--- |
| **1** | [backend/tools/article.go](file:///Volumes/ORICO/Desktop_backup/test/mini-agent/backend/tools/article.go) | **新建** | 声明 `delete_article` 工具，并定义高危工具判断方法 |
| **2** | [backend/controllers/chat.go](file:///Volumes/ORICO/Desktop_backup/test/mini-agent/backend/controllers/chat.go) | **修改** | 注入高危拦截网关，遇到删除工具时下发 `require_approval` |
| **3** | [backend/main.go](file:///Volumes/ORICO/Desktop_backup/test/mini-agent/backend/main.go) | **修改** | 注册新路由：`POST /v1/chat/approve` 处理人类审批点击 |
| **4** | [frontend/src/views/chat/index.vue](file:///Volumes/ORICO/Desktop_backup/test/mini-agent/frontend/src/views/chat/index.vue) | **修改** | 增加警示审批卡片、批准/拒绝按钮及点击调用逻辑 |

---

## 二、Step 1：后端创建高危工具与权限分类

在 `backend/tools/` 目录下新建一个专门处理文章的高危工具文件：

👉 **新建文件**：[backend/tools/article.go](file:///Volumes/ORICO/Desktop_backup/test/mini-agent/backend/tools/article.go)

```go
package tools

import (
	"encoding/json"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
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
```

紧接着，将这个新工具加入到 `tools` 包的总声明清单中：  
打开之前建好的 [backend/tools/waether.go](file:///Volumes/ORICO/Desktop_backup/test/mini-agent/backend/tools/waether.go)：

1. 在 `GetToolDefinitions()` 数组里追加 `delete_article` 的描述；
2. 在 `ExecuteTool()` 路由里加上 `case "delete_article"`。

👉 **修改 `waether.go` 代码片段**：
```go
// 追加工具定义
func GetToolDefinitions() []openai.Tool {
	return []openai.Tool{
		// 1. 原有的查天气工具
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
		// 2. 👈 新增高危工具：删除文章
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "delete_article",
				Description: "根据文章 ID 彻底删除系统中的文章或笔记。此操作具有严重破坏性且不可逆，请谨慎调用！",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"article_id": {
							Type:        jsonschema.String,
							Description: "要删除的文章ID，如：88、101",
						},
					},
					Required: []string{"article_id"},
				},
			},
		},
	}
}

// 追加工具分发路由
func ExecuteTool(name string, args string) string {
	switch name {
	case "get_weather":
		return GetWeather(args)
	case "delete_article": // 👈 新增分发
		return DeleteArticle(args)
	default:
		return fmt.Sprintf(`{"error": "未知工具: %s"}`, name)
	}
}
```

---

## 三、Step 2：后端拦截器与审批任务缓存

打开 [backend/controllers/chat.go](file:///Volumes/ORICO/Desktop_backup/test/mini-agent/backend/controllers/chat.go)：

### 1. 结构体扩展（支持 TaskID）
把 `StreamChunk` 加上 `TaskID`：
```go
type StreamChunk struct {
	Content    string `json:"content,omitempty"`
	Reasoning  string `json:"reasoning,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	ToolArgs   string `json:"tool_args,omitempty"`
	ToolResult string `json:"tool_result,omitempty"`
	ToolStatus string `json:"tool_status,omitempty"` // calling | done | require_approval
	TaskID     string `json:"task_id,omitempty"`     // 👈 新增：审批任务唯一标识
	Error      string `json:"error,omitempty"`
}
```

### 2. 内存任务暂存池（PendingTasks）
在 `controllers/chat.go` 文件顶部定义一个全局并发安全的任务暂存池：
```go
// 暂存挂起的审批任务
type PendingTask struct {
	TaskID     string
	ToolName   string
	ToolArgs   string
	ToolCallID string
	Messages   []openai.ChatCompletionMessage
	Model      string
}

// 内存任务池（生产环境可换成 Redis）
var pendingTasksMap = sync.Map{}
```

### 3. 高危拦截逻辑
在 `chat.go` 收集完 `toolArguments` 后（大约第 160 行附近），加入**高危工具判断**：
```go
	fullArgs := toolArguments.String()

	// 🛡️ 核心拦截器：判断是否为高危工具
	if tools.IsDangerousTool(toolName) {
		taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())

		// 将当前上下文暂存起来，等待人类决策
		pendingTasksMap.Store(taskID, PendingTask{
			TaskID:     taskID,
			ToolName:   toolName,
			ToolArgs:   fullArgs,
			ToolCallID: toolCallID,
			Messages:   chatMsgs,
			Model:      model,
		})

		// 向前端下发【等待审批】卡片，并结束当前这轮 SSE
		sendChunk(StreamChunk{
			ToolName:   toolName,
			ToolArgs:   fullArgs,
			ToolStatus: "require_approval",
			TaskID:     taskID,
		})

		fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
		c.Writer.Flush()
		return
	}

	// 下面是原有的只读工具自动执行逻辑（get_weather）...
```

---

## 四、Step 3：后端增加审批决策接口（POST /v1/chat/approve）

在 [backend/controllers/chat.go](file:///Volumes/ORICO/Desktop_backup/test/mini-agent/backend/controllers/chat.go) 中新增一个 `Approve` 处理函数：

```go
type ApproveRequest struct {
	TaskID string `json:"task_id" binding:"required"`
	Agree  bool   `json:"agree"` // true: 批准, false: 拒绝
}

// Approve 处理前端点击【批准】或【拒绝】的二次流式会话
func (ctrl *ChatController) Approve(c *gin.Context) {
	var req ApproveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	// 1. 从暂存池中取出之前的任务上下文
	val, ok := pendingTasksMap.Load(req.TaskID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到待审批的任务或任务已过期"})
		return
	}
	pendingTasksMap.Delete(req.TaskID) // 取出后消费掉
	task := val.(PendingTask)

	apiKey := strings.Trim(strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY")), "\"")
	baseURL := strings.Trim(strings.TrimSpace(os.Getenv("DEEPSEEK_BASE_URL")), "\"")
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	config := openai.DefaultConfig(apiKey)
	config.BaseURL = baseURL
	client := openai.NewClientWithConfig(config)

	// 2. 根据人类的决策，决定真实执行还是取消
	var toolResult string
	if req.Agree {
		// 人类点击了【批准执行】：真正去调用本地删除函数！
		toolResult = tools.ExecuteTool(task.ToolName, task.ToolArgs)
	} else {
		// 人类点击了【拒绝操作】：不执行删除，并向大模型反馈用户已拒绝！
		toolResult = `{"status": "rejected", "message": "用户在审批卡片中点击了【拒绝】，已取消本次高危操作！"}`
	}

	// 3. 设置 SSE 响应头，向前端流式输出最终结果
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	sendChunk := func(chunk StreamChunk) {
		bytes, _ := json.Marshal(chunk)
		fmt.Fprintf(c.Writer, "data: %s\n\n", bytes)
		c.Writer.Flush()
	}

	// 4. 将审批后的工具结果追加进历史上下文
	chatMsgs := append(task.Messages, openai.ChatCompletionMessage{
		Role: openai.ChatMessageRoleAssistant,
		ToolCalls: []openai.ToolCall{
			{
				ID:   task.ToolCallID,
				Type: openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      task.ToolName,
					Arguments: task.ToolArgs,
				},
			},
		},
	})
	chatMsgs = append(chatMsgs, openai.ChatCompletionMessage{
		Role:       openai.ChatMessageRoleTool,
		ToolCallID: task.ToolCallID,
		Name:       task.ToolName,
		Content:    toolResult,
	})

	// 5. 调大模型做最终人类语言汇总
	stream, err := client.CreateChatCompletionStream(c.Request.Context(), openai.ChatCompletionRequest{
		Model:    task.Model,
		Messages: chatMsgs,
		Stream:   true,
	})
	if err != nil {
		sendChunk(StreamChunk{Error: "调用模型失败: " + err.Error()})
		return
	}
	defer stream.Close()

	for {
		resp, err := stream.Recv()
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

	fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
	c.Writer.Flush()
}
```

最后在 [backend/main.go](file:///Volumes/ORICO/Desktop_backup/test/mini-agent/backend/main.go) 注册路由：
```go
v1.POST("/chat/stream", chatController.Stream)
v1.POST("/chat/approve", chatController.Approve) // 👈 新增这一行
```

---

## 五、Step 4：前端 Vue3 审批卡片与按钮交互

打开 [frontend/src/views/chat/index.vue](file:///Volumes/ORICO/Desktop_backup/test/mini-agent/frontend/src/views/chat/index.vue)：

### 1. 结构体与工具字典扩展
在 `ToolInfo` 接口中增加 `taskId` 和状态扩展：
```typescript
interface ToolInfo {
  name: string;
  args?: string;
  result?: string;
  status: "calling" | "done" | "require_approval" | "approved" | "rejected";
  taskId?: string; // 👈 记录待审批的任务 ID
}

const toolNameMap: Record<string, string> = {
  get_weather: "实时天气查询",
  delete_article: "删除文章/高危数据" // 👈 新增中文映射
};
```

### 2. 在 SSE 接收中提取 `task_id`
在 `handleSend` 方法里解析 `parsed.tool_name` 的地方加上：
```typescript
if (parsed.tool_name) {
  targetMsg.tool = {
    name: parsed.tool_name,
    args: parsed.tool_args || targetMsg.tool?.args,
    result: parsed.tool_result || targetMsg.tool?.result,
    status: parsed.tool_status || "calling",
    taskId: parsed.task_id || targetMsg.tool?.taskId // 👈 绑定 taskId
  };
  scrollToBottom();
}
```

### 3. 模板中渲染【高危警示卡片】与按钮
将工具卡片升级为支持审批的形态：
```html
<!-- ⚙️ 外部工具调用状态卡片 -->
<div v-if="msg.tool" class="tool-call-box" :class="msg.tool.status">
  <!-- A. 处于待审批状态时的危险警告卡片 -->
  <div v-if="msg.tool.status === 'require_approval'" class="approval-card">
    <div class="approval-header">
      <span class="warning-icon">⚠️</span>
      <div class="approval-title-wrap">
        <span class="approval-title">高危操作拦截：AI 申请调用【{{ getToolDisplayName(msg.tool.name) }}】</span>
        <span class="approval-sub">此操作具有破坏性，必须经过人类授权确认后方可执行。</span>
      </div>
    </div>
    <div class="tool-call-body" style="background: rgba(0,0,0,0.02)">
      <div class="tool-item">
        <span class="item-label">目标参数:</span>
        <code class="item-code">{{ msg.tool.args }}</code>
      </div>
    </div>
    <div class="approval-actions">
      <el-button type="danger" size="small" @click="handleApprovalClick(msg, true)">
        批准执行 (Approve)
      </el-button>
      <el-button size="small" @click="handleApprovalClick(msg, false)">
        拒绝操作 (Reject)
      </el-button>
    </div>
  </div>

  <!-- B. 已经批准/拒绝或普通工具时的可折叠卡片 -->
  <details v-else class="tool-details" :open="msg.tool.status === 'calling'">
    <summary class="tool-summary">
      <div class="tool-summary-left">
        <span v-if="msg.tool.status === 'calling'" class="tool-spin">⚙️</span>
        <span v-else-if="msg.tool.status === 'approved'" style="color: #e6a23c">⚡</span>
        <span v-else-if="msg.tool.status === 'rejected'" style="color: #909399">🚫</span>
        <span v-else class="tool-check">✅</span>

        <span class="tool-label">
          {{ msg.tool.status === 'approved' ? '已批准执行' : msg.tool.status === 'rejected' ? '已人工拒绝' : '已调用工具' }}：
        </span>
        <span class="tool-name">{{ getToolDisplayName(msg.tool.name) }}</span>
      </div>
      <span class="tool-toggle-hint">详情 ›</span>
    </summary>
    <div class="tool-call-body">
      <div v-if="msg.tool.args" class="tool-item">
        <span class="item-label">入参:</span>
        <code class="item-code">{{ msg.tool.args }}</code>
      </div>
      <div v-if="msg.tool.result" class="tool-item">
        <span class="item-label">返回:</span>
        <code class="item-code result">{{ msg.tool.result }}</code>
      </div>
    </div>
  </details>
</div>
```

### 4. 编写点击审批按钮的请求函数
在 `<script setup>` 中新增审批函数 `handleApprovalClick`：
```typescript
const handleApprovalClick = async (msg: ChatMessage, agree: boolean) => {
  if (!msg.tool || !msg.tool.taskId) return;
  const taskId = msg.tool.taskId;

  // 1. 更新卡片状态
  msg.tool.status = agree ? "approved" : "rejected";
  isStreaming.value = true;

  try {
    const response = await fetch("/v1/chat/approve", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ task_id: taskId, agree })
    });

    if (!response.body) throw new Error("无响应流");
    const reader = response.body.getReader();
    const decoder = new TextDecoder("utf-8");
    let buffer = "";

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n\n");
      buffer = lines.pop() || "";

      for (const line of lines) {
        const trimmed = line.trim();
        if (trimmed.startsWith("data:")) {
          const dataStr = trimmed.slice(5).trim();
          if (dataStr === "[DONE]") break;
          try {
            const parsed = JSON.parse(dataStr);
            if (parsed.content) {
              msg.content += parsed.content;
              scrollToBottom();
            }
          } catch {}
        }
      }
    }
  } catch (error: any) {
    ElMessage.error(error.message || "审批处理失败");
  } finally {
    isStreaming.value = false;
    scrollToBottom();
  }
};
```

### 5. 补充高危卡片 CSS 样式
在 `<style scoped>` 中追加：
```css
/* 审批卡片警告样式 */
.tool-call-box.require_approval {
  border: 1px solid #fed7aa;
  background: #fff7ed;
  box-shadow: 0 2px 10px rgba(234, 88, 12, 0.08);
}

.approval-card {
  padding: 12px 14px;
}

.approval-header {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin-bottom: 8px;
}

.warning-icon {
  font-size: 18px;
  line-height: 1;
}

.approval-title-wrap {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.approval-title {
  font-weight: 600;
  color: #c2410c;
  font-size: 13px;
}

.approval-sub {
  font-size: 11.5px;
  color: #ea580c;
}

.approval-actions {
  display: flex;
  gap: 10px;
  margin-top: 10px;
  padding-top: 8px;
  border-top: 1px dashed rgba(234, 88, 12, 0.2);
}
```

---

## 六、连调验证与大白话面试复盘

完成修改并重启 Go 后端后：
1. 在浏览器里提问普通问题：“**今天杭州天气怎么样？**”
   - 👉 验证正常工具是否依然静默执行；
2. 在浏览器里下达高危命令：“**请帮我把 ID 为 88 的文章删掉**”；
   - 👉 观察页面是否弹出醒目的 **【高危操作拦截】** 卡片与两个按钮！
   - 点击 **【拒绝】**：看 AI 是否温和告知已取消；
   - 再发一次点击 **【批准】**：看 AI 是否调取删除逻辑并反馈成功！

---

💡 **开始行动吧！**  
按照本文档的 **Step 1 到 Step 4** 依次编码，有任何报错或需要梳理逻辑的地方随时唤醒我！
