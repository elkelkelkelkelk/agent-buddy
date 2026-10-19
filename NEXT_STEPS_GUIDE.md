# Mini-Agent 实战路线：下一步详细开发指南

> 本指南针对**前端开发者的 AI Agent 实战求职路线**制定，基于当前已就绪的脚手架（前端 Pure Admin + 后端 Go Gin），从零实现一个具备商业级体验的 **MVP 1.0 流式智能对话与打字机闭环**。

---

## 📌 当前状态回顾
- ✅ **前端脚手架**：`pure-admin-thin` 已克隆并在 `http://localhost:8848`（或对应 Vite 端口）运行。
- ✅ **后端脚手架**：`gin-boilerplate` 已完成 Go toolchain 配置、PostgreSQL 与 Redis 依赖就绪，在 `http://localhost:9000` 正常运行。

---

## 🎯 下一步核心目标：阶段一（MVP 1.0）流式对话打字机闭环

### 为什么要先做这个？
1. **Agent 的一切交互基础**：所有商业级 AI Agent（如 ChatGPT、Dify、Coze）最底层的交互都是基于 **SSE（Server-Sent Events）** 的流式数据管道。
2. **简历核心亮点**：深入理解分块流式传输、Markdown 动态渲染防抖动、长连接保活与异常中断机制。

```
┌─────────────────┐       POST /v1/chat/stream       ┌──────────────────┐       Stream Request       ┌─────────────────┐
│                 │ ───────────────────────────────> │                  │ ─────────────────────────> │                 │
│ Pure Admin 前端 │                                  │  Go Gin 后端服务  │                            │ DeepSeek/OpenAI │
│  (Chat 页面)    │ <=============================== │ (流式中继 Flush) │ <========================= │   大模型 API    │
└─────────────────┘       text/event-stream (SSE)    └──────────────────┘          SSE Tokens        └─────────────────┘
```

---

## 📋 详细任务拆解（按步骤执行）

### 任务 1：打通前后端通信网络（Vite 代理配置）
让前端通过相对路径请求后端，规避跨域并模拟生产环境代理配置。

- **目标文件**：`frontend/vite.config.ts`
- **操作内容**：
  在 Vite 的 `server.proxy` 中添加后端代理转发规则：
  ```ts
  '/api': {
    target: 'http://localhost:9000',
    changeOrigin: true,
    rewrite: (path) => path.replace(/^\/api/, '')
  }
  ```
- **验收标准**：前端通过 `fetch('/api/v1/...')` 可以直接收到 Go 后端的响应。

---

### 任务 2：后端实现 SSE 流式聊天接口
在 Go 后端引入标准大模型 SDK，编写支持 `text/event-stream` 的控制器。

1. **引入大模型 SDK（推荐使用标准 OpenAI 兼容库）**：
   ```bash
   cd backend
   go get github.com/sashabaranov/go-openai
   ```
2. **环境变量配置（`.env`）**：
   添加大模型厂商的 API Key 和 BaseURL（可直接对接 DeepSeek、通义千问、Kimi 或 Ollama 本地模型）：
   ```env
   LLM_API_KEY="sk-xxxxxxxxxxxxxxxxxxxxxxxx"
   LLM_BASE_URL="https://api.deepseek.com/v1"
   LLM_MODEL="deepseek-chat"
   ```
3. **编写聊天控制器（`controllers/chat.go`）**：
   - 接收前端请求体：`{ "messages": [{ "role": "user", "content": "你好" }] }`
   - 设置 SSE 响应头：
     ```go
     c.Header("Content-Type", "text/event-stream")
     c.Header("Cache-Control", "no-cache")
     c.Header("Connection", "keep-alive")
     ```
   - 监听模型返回流（Stream Channel），每次收到 Token 立即向前端 `c.Writer.Flush()`。
4. **注册路由**：
   在 `main.go` 中注册 `v1.POST("/chat/stream", chatController.Stream)`。

---

### 任务 3：前端搭建 AI 对话页面（Pure Admin）
在现有 Pure Admin 后台系统中新增一个独立的 AI 对话界面。

1. **新增路由与菜单配置**：
   - 在 `frontend/src/router/modules/` 下添加 `chat.ts` 路由配置。
   - 页面路径：`src/views/chat/index.vue`。
2. **安装流式请求与 Markdown 渲染库**：
   ```bash
   cd frontend
   pnpm add @microsoft/fetch-event-source markdown-it highlight.js
   ```
3. **页面功能实现**：
   - **消息列表区**：区分 User 气泡与 Assistant 气泡，支持 Markdown 代码高亮。
   - **流式接收逻辑**：使用 `@microsoft/fetch-event-source` 消费后端的 SSE 流，实时将字追加到当前消息气泡中。
   - **打字机体验优化**：
     - 发送时锁定输入框，展示“思考中...”指示器。
     - 内容生成过程中自动平滑滚动到底部（吸底）。
     - 提供“停止生成”按钮（通过 `AbortController` 中断连接）。

---

## 🚀 验收标准与效果演示
完成上述步骤后，系统将达成以下效果：
1. 登录 Pure Admin 后台，左侧菜单出现 **“AI 智能助手”**。
2. 进入页面，在输入框输入任意问题（如：“请用 Go 语言写一个快速排序”）。
3. 页面立即展示思考状态，随后像 ChatGPT 一样**一个字一个字平滑打出回答**，并带有漂亮的代码高亮。
4. 终端可以观察到 Go 后端在极低内存占用（< 30MB）下稳定流式转发 Token。

---

## 🔮 展望后续进阶（预告）
- **阶段二：Tool Calling（函数调用）**
  模型自主决定调用 Go 函数（例如：查询本地数据库、查询实时天气、读写文件）。
- **阶段三：HITL（Human-in-the-Loop 人机协同审批）**
  高危操作（如删除数据库记录、转账）时，前端弹出审批确认卡片，待人工点击确认后模型继续执行下一步。
