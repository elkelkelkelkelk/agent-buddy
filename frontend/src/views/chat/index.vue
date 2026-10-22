<script setup lang="ts">
import { ref, nextTick, onMounted } from "vue";
import MarkdownIt from "markdown-it";
import { ElMessage } from "element-plus";

defineOptions({
  name: "AiChat"
});

//1.定义工具状态数据接口
interface ToolInfo {
  name: string;
  args?: string;
  result?: string;
  status: "calling" | "done" | "require_approval" | "approved" | "rejected";
  taskId?: string;    // 审批任务唯一 ID
}
// 
interface ChatMessage {
  id: string;
  role: "user" | "assistant" | "system";
  content: string;
  reasoning?: string;
  tool?: ToolInfo;
  timestamp: string;
}

const md = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: true
});

// 工具名称友好中文别名映射
const toolNameMap: Record<string, string> = {
  get_weather: "实时天气查询",
  delete_article: "删除文章/高危数据"
};

const getToolDisplayName = (name: string) => toolNameMap[name] || name;

const inputPrompt = ref("");
const isStreaming = ref(false);
const chatListRef = ref<HTMLElement | null>(null);
let abortController: AbortController | null = null;

const messages = ref<ChatMessage[]>([
  {
    id: "welcome",
    role: "assistant",
    content: "👋 你好！我们随时开始！",
    timestamp: new Date().toLocaleTimeString()
  }
]);

const scrollToBottom = async () => {
  await nextTick();
  if (chatListRef.value) {
    chatListRef.value.scrollTop = chatListRef.value.scrollHeight;
  }
};

const handleSend = async () => {
  const text = inputPrompt.value.trim();
  if (!text) return;
  if (isStreaming.value) return;

  const userMsgId = Date.now().toString();
  const assistantMsgId = (Date.now() + 1).toString();

  // 添加用户消息
  messages.value.push({
    id: userMsgId,
    role: "user",
    content: text,
    timestamp: new Date().toLocaleTimeString()
  });

  inputPrompt.value = "";
  isStreaming.value = true;
  await scrollToBottom();

  // 添加占位回复消息
  messages.value.push({
    id: assistantMsgId,
    role: "assistant",
    content: "",
    reasoning: "",
    timestamp: new Date().toLocaleTimeString()
  });

  // 构造历史上下文
  const payloadMessages = messages.value
    .filter(m => m.id !== assistantMsgId && m.id !== "welcome")
    .map(m => ({
      role: m.role,
      content: m.content
    }));

  abortController = new AbortController();

  try {
    const response = await fetch("/v1/chat/stream", {
      method: "POST",
      headers: {
        "Content-Type": "application/json"
      },
      body: JSON.stringify({ messages: payloadMessages }),
      signal: abortController.signal
    });

    if (!response.ok) {
      const errData = await response.json().catch(() => null);
      throw new Error(errData?.error || `请求失败 (${response.status})`);
    }

    if (!response.body) {
      throw new Error("浏览器不支持 ReadableStream");
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder("utf-8");
    let buffer = "";

    const targetMsg = messages.value.find(m => m.id === assistantMsgId);

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
          if (dataStr === "[DONE]") {
            break;
          }
          try {
            const parsed = JSON.parse(dataStr);
            if (parsed.error) {
              ElMessage.error(parsed.error);
            } else if (targetMsg) {
              //1.思考过程收集
              if (parsed.reasoning) {
                targetMsg.reasoning = (targetMsg.reasoning || "") + parsed.reasoning;
                scrollToBottom();
              }
              //2.工具调用状态解析
              if (parsed.tool_name) {
                targetMsg.tool = {
                  name: parsed.tool_name,
                  args: parsed.tool_args || targetMsg.tool?.args,
                  result: parsed.tool_result || targetMsg.tool?.result,
                  status: parsed.tool_status || "calling",
                  taskId: parsed.task_id || targetMsg.tool?.taskId
                }
                scrollToBottom()
              }
              //3.正文回答收集
              if (parsed.content) {
                targetMsg.content += parsed.content;
                scrollToBottom();
              }
            }
          } catch {
            // 非 JSON 数据
          }
        }
      }
    }
  } catch (error: any) {
    if (error.name === "AbortError") {
      ElMessage.info("已停止生成");
    } else {
      ElMessage.error(error.message || "流式请求发生错误");
      const targetMsg = messages.value.find(m => m.id === assistantMsgId);
      if (targetMsg && !targetMsg.content) {
        targetMsg.content = `⚠️ 请求异常：${error.message}`;
      }
    }
  } finally {
    isStreaming.value = false;
    abortController = null;
    scrollToBottom();
  }
};

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
          } catch { }
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

const handleStop = () => {
  if (abortController) {
    abortController.abort();
  }
};

const handleKeyDown = (e: KeyboardEvent) => {
  if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault();
    handleSend();
  }
};

const handleClear = () => {
  messages.value = [messages.value[0]];
};

onMounted(() => {
  scrollToBottom();
});
</script>

<template>
  <div class="chat-container">
    <!-- 头部状态栏 -->
    <header class="chat-header">
      <div class="header-left">
        <div class="model-badge">DeepSeek (阿里云百炼)</div>
        <span class="header-title">AI 智能会话工作台</span>
      </div>
      <div class="header-right">
        <el-button size="small" text @click="handleClear">清空会话</el-button>
      </div>
    </header>

    <!-- 消息列表区 -->
    <main ref="chatListRef" class="chat-messages">
      <div v-for="msg in messages" :key="msg.id" class="message-row" :class="msg.role">
        <div class="avatar">
          <span v-if="msg.role === 'user'">👤</span>
          <span v-else>🤖</span>
        </div>
        <div class="bubble-wrapper">
          <div class="bubble-info">
            <span class="role-name">{{ msg.role === 'user' ? '我' : 'DeepSeek' }}</span>
            <span class="time">{{ msg.timestamp }}</span>
          </div>
          <div class="bubble-content">
            <!-- 深度思考过程折叠框（DeepSeek / ChatGPT 同款：生成时展开，输出答案后自动收起） -->
            <details v-if="msg.reasoning" class="thought-box" :open="isStreaming && !msg.content">
              <summary class="thought-summary">
                <div class="thought-summary-left">
                  <span>🧠</span>
                  <span>{{ isStreaming && !msg.content ? '正在深度思考中...' : '已完成深度思考' }}</span>
                </div>
                <span class="thought-toggle-hint">{{ isStreaming && !msg.content ? '思考中...' : '展开思考 ›' }}</span>
              </summary>
              <div class="thought-text">{{ msg.reasoning }}</div>
            </details>
            <!-- ⚙️ 外部工具调用状态卡片（ChatGPT / 商业级同款可折叠设计） -->
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
                      {{ msg.tool.status === 'approved' ? '已批准执行' : msg.tool.status === 'rejected' ? '已人工拒绝' : '已调用工具'
                      }}：
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
            <!-- 正文内容 -->
            <div v-if="msg.content" class="markdown-body" v-html="md.render(msg.content)" />
            <!-- 正在思考/等待指示器 -->
            <div v-else-if="isStreaming && msg.role === 'assistant'" class="typing-indicator">
              <span class="dot" />
              <span class="dot" />
              <span class="dot" />
            </div>
          </div>
        </div>
      </div>
    </main>

    <!-- 底部输入控制台 -->
    <footer class="chat-footer">
      <div class="input-box-wrapper">
        <el-input v-model="inputPrompt" type="textarea" :rows="3" resize="none"
          placeholder="输入你的问题，按 Enter 发送，Shift + Enter 换行..." :disabled="isStreaming" @keydown="handleKeyDown" />
        <div class="action-bar">
          <span class="hint-text">由 Go Gin + SSE 流式引擎驱动</span>
          <div class="btn-group">
            <el-button v-if="isStreaming" type="danger" plain @click="handleStop">
              停止生成
            </el-button>
            <el-button v-else type="primary" :disabled="!inputPrompt.trim()" @click="handleSend">
              发送消息
            </el-button>
          </div>
        </div>
      </div>
    </footer>
  </div>
</template>

<style scoped>
.chat-container {
  display: flex;
  flex-direction: column;
  height: calc(100vh - 120px);
  background: var(--el-bg-color-overlay, #ffffff);
  border-radius: 12px;
  overflow: hidden;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.05);
  border: 1px solid var(--el-border-color-lighter, #ebeef5);
}

.chat-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 20px;
  border-bottom: 1px solid var(--el-border-color-lighter, #f0f0f0);
  background: var(--el-bg-color, #fafafa);
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.model-badge {
  background: linear-gradient(135deg, #0284c7, #2563eb);
  color: #fff;
  font-size: 12px;
  font-weight: 600;
  padding: 3px 10px;
  border-radius: 20px;
}

.header-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--el-text-color-primary, #303133);
}

.chat-messages {
  flex: 1;
  overflow-y: auto;
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 20px;
  scroll-behavior: smooth;
}

.message-row {
  display: flex;
  gap: 12px;
  max-width: 85%;
}

.message-row.user {
  align-self: flex-end;
  flex-direction: row-reverse;
}

.message-row.assistant {
  align-self: flex-start;
}

.avatar {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  background: var(--el-fill-color-light, #f2f3f5);
  flex-shrink: 0;
}

.bubble-wrapper {
  display: flex;
  flex-direction: column;
}

.message-row.user .bubble-wrapper {
  align-items: flex-end;
}

.bubble-info {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
  font-size: 12px;
  color: var(--el-text-color-secondary, #909399);
}

.bubble-content {
  padding: 12px 16px;
  border-radius: 12px;
  font-size: 14px;
  line-height: 1.6;
  word-break: break-word;
}

.message-row.user .bubble-content {
  background: #409eff;
  color: #ffffff;
  border-bottom-right-radius: 2px;
}

.message-row.assistant .bubble-content {
  background: var(--el-fill-color-light, #f5f7fa);
  color: var(--el-text-color-primary, #303133);
  border-bottom-left-radius: 2px;
  border: 1px solid var(--el-border-color-lighter, #e4e7ed);
}

.thought-box {
  margin-bottom: 12px;
  background: #f8fafc;
  border-left: 3px solid #38bdf8;
  border-radius: 6px;
  font-size: 12.5px;
  color: #64748b;
  overflow: hidden;
  transition: all 0.3s ease;
}

.thought-summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  cursor: pointer;
  user-select: none;
  list-style: none;
  font-weight: 600;
  color: #0284c7;
}

.thought-summary::-webkit-details-marker {
  display: none;
}

.thought-summary-left {
  display: flex;
  align-items: center;
  gap: 6px;
}

.thought-toggle-hint {
  font-size: 11.5px;
  color: #94a3b8;
  font-weight: normal;
  transition: color 0.2s ease;
}

.thought-summary:hover .thought-toggle-hint {
  color: #0284c7;
}

.thought-text {
  padding: 0 12px 10px 12px;
  white-space: pre-wrap;
  line-height: 1.5;
  color: #475569;
  border-top: 1px dashed rgba(56, 189, 248, 0.2);
  margin-top: 4px;
  padding-top: 8px;
}

.markdown-body :deep(p) {
  margin: 0 0 8px 0;
}

.markdown-body :deep(p:last-child) {
  margin-bottom: 0;
}

.markdown-body :deep(pre) {
  background: #1e1e1e;
  color: #d4d4d4;
  padding: 12px;
  border-radius: 8px;
  overflow-x: auto;
  margin: 8px 0;
}

.markdown-body :deep(code) {
  font-family: Menlo, Monaco, Consolas, "Courier New", monospace;
  font-size: 13px;
}

.typing-indicator {
  display: flex;
  gap: 4px;
  align-items: center;
  height: 20px;
}

.typing-indicator .dot {
  width: 6px;
  height: 6px;
  background-color: var(--el-color-primary, #409eff);
  border-radius: 50%;
  animation: bounce 1.4s infinite ease-in-out both;
}

.typing-indicator .dot:nth-child(1) {
  animation-delay: -0.32s;
}

.typing-indicator .dot:nth-child(2) {
  animation-delay: -0.16s;
}

@keyframes bounce {

  0%,
  80%,
  100% {
    transform: scale(0);
  }

  40% {
    transform: scale(1);
  }
}

.chat-footer {
  padding: 16px 20px;
  border-top: 1px solid var(--el-border-color-lighter, #f0f0f0);
  background: var(--el-bg-color, #ffffff);
}

.input-box-wrapper {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.action-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.hint-text {
  font-size: 12px;
  color: var(--el-text-color-placeholder, #c0c4cc);
}

.btn-group {
  display: flex;
  gap: 8px;
}

/* 工具调用卡片容器（ChatGPT 同款精致胶囊折叠卡） */
.tool-call-box {
  margin-bottom: 12px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  font-size: 13px;
  transition: all 0.3s ease;
  overflow: hidden;
}

.tool-call-box.calling {
  border-color: #93c5fd;
  background: #eff6ff;
  box-shadow: 0 0 10px rgba(59, 130, 246, 0.1);
}

.tool-call-box.done {
  border-color: #bbf7d0;
  background: #f0fdf4;
}

.tool-details {
  width: 100%;
}

.tool-summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 14px;
  cursor: pointer;
  user-select: none;
  list-style: none;
}

.tool-summary::-webkit-details-marker {
  display: none;
}

.tool-summary-left {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 500;
}

.tool-toggle-hint {
  font-size: 11.5px;
  color: #94a3b8;
  transition: color 0.2s ease;
}

.tool-summary:hover .tool-toggle-hint {
  color: #0284c7;
}

.tool-spin {
  display: inline-block;
  animation: spin 2s linear infinite;
  font-size: 14px;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }

  to {
    transform: rotate(360deg);
  }
}

.tool-label {
  color: #64748b;
  font-size: 12.5px;
}

.tool-name {
  color: #0f172a;
  font-weight: 600;
  background: rgba(0, 0, 0, 0.05);
  padding: 2px 7px;
  border-radius: 4px;
  font-size: 12px;
}

.tool-call-body {
  padding: 8px 14px 10px 14px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  border-top: 1px dashed rgba(0, 0, 0, 0.08);
  background: rgba(255, 255, 255, 0.6);
}

.tool-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: 12px;
}

.item-label {
  color: #64748b;
  flex-shrink: 0;
  font-weight: 500;
}

.item-code {
  font-family: Menlo, Monaco, Consolas, monospace;
  background: rgba(0, 0, 0, 0.04);
  padding: 3px 6px;
  border-radius: 4px;
  word-break: break-all;
  color: #334155;
}

.item-code.result {
  color: #15803d;
}

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
</style>
