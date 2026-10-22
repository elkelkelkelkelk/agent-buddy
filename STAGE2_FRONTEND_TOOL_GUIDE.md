# 阶段二·实战指南：前端工具调用可视化卡片与状态机实现

> **前言**：在上一阶段，我们已经在 Go 后端成功打通了工具调用的两次握手，并在终端中看到了数据流：
> `{"tool_name": "get_weather", "tool_status": "calling"}` ➔ `{"tool_status": "done", "tool_result": "..."}`。
> 本步骤的目标是：**在 Pure Admin 前端 Vue3 界面中，将这些冰冷的数据流转变成用户可感知的【三段式高颜值智能体卡片】！**

---

## 目录
- [一、前端交互设计蓝图与状态机原理](#一前端交互设计蓝图与状态机原理)
- [二、大厂高频面试真题与深度考点剖析](#二大厂高频面试真题与深度考点剖析)
- [三、动手实操四步法（Step-by-Step）](#三动手实操四步法step-by-step)
  - [步骤 1：扩展 TypeScript 消息数据模型](#步骤-1扩展-typescript-消息数据模型)
  - [步骤 2：在 SSE 循环中解析工具状态帧](#步骤-2在-sse-循环中解析工具状态帧)
  - [步骤 3：在 Template 中新增工具交互卡片](#步骤-3在-template-中新增工具交互卡片)
  - [步骤 4：添加卡片质感与微动效样式](#步骤-4添加卡片质感与微动效样式)
- [四、最终效果验收与自测](#四最终效果验收与自测)

---

## 一、前端交互设计蓝图与状态机原理

### 1.1 为什么不能直接全部当 Markdown 文本展示？
传统的问答界面只有两段：用户提问 ➔ AI 回答。  
但对于 Agent 而言，一次回答包含三个不同维度的认知层级：

```
┌────────────────────────────────────────────────────────┐
│ 🧠 深度思考折叠块 (Reasoning)                          │
│    "用户想查杭州天气，我需要调用 get_weather 工具..."   │
└────────────────────────────────────────────────────────┘
                           ↓
┌────────────────────────────────────────────────────────┐
│ ⚙️ 外部工具调用卡片 (Tool Status Card)                 │
│    [动态齿轮 旋转] 正在调用工具: get_weather (入参: 杭州) │
│    [绿色对勾 完成] 执行成功，返回: 26℃ 晴朗 东南风 2级   │
└────────────────────────────────────────────────────────┘
                           ↓
┌────────────────────────────────────────────────────────┐
│ 💬 最终生成的回答 (Final Answer Markdown)               │
│    "杭州今天天气晴朗，气温约 26℃，适合出行..."          │
└────────────────────────────────────────────────────────┘
```

### 1.2 前端状态机流转设计
单条消息中的 `tool` 对象具有两个关键状态：
- **`calling`（执行中）**：展示 Loading 旋转图标、工具名称、提取出的入参，背景呈现淡蓝色微光呼吸效果；
- **`done`（已完成）**：展示绿色对勾图标，支持展开查看工具返回的原生数据（`tool_result`）。

---

## 二、大厂高频面试真题与大白话答题技巧（附记忆小抄）

### 🎤 面试题 1：前端要做很多卡片（文本、天气卡片、审批按钮），怎么写才不会把组件写得又臭又长？
> **面试官真实在问**：你的 Vue 组件设计规范吗？是不是把所有逻辑堆在一个 `index.vue` 里面了？

* **3 个词记忆小抄**：**数据分开存、模板分插槽、按类型动态渲染**
* **🗣️ 大白话回答**：
  > “我们在设计消息对象时，就没把所有东西混在一坨字符串里。
  > 一条消息里，我把它的**思考过程（reasoning）**、**工具调用（tool）**和**最终正文（content）**拆成独立的字段。
  > 
  > 在 Vue 界面里也是按三块来渲染：上面是思考折叠框，中间是工具状态卡，下面是 Markdown 文本。
  > 以后如果工具很多（比如有股票图表、有审批确认弹窗），我们就可以根据工具名字去动态加载不同的独立子组件，主聊天页面只负责排版，这样代码就不会越写越乱、越来越难维护。”

---

### 🎤 面试题 2：流式打字机（SSE）字吐得飞快，前端怎么保证不卡顿、滚动条不乱跳？
> **面试官真实在问**：你有前端流式渲染的实战踩坑经验吗？

* **3 个词记忆小抄**：**攒一攒再更新、判断用户有没有往上滑**
* **🗣️ 大白话回答**：
  > “大模型字吐得很快的时候，一秒钟可能有三四十个小碎片过来。如果每来一个字我都去改 Vue 的响应式数据，页面很容易掉帧发热。
  > 
  > 我们做前端优化的时候主要注意两点：
  > 1. **加个微小节流或者 RAF**：把每秒几十次高频渲染稍微合并一下，保证在 60 帧以内平滑追加，肉眼看着完全是丝滑打字，但 CPU 消耗会低很多；
  > 2. **自动滚动不能太霸道**：默认情况下有新字出来，页面会自动滚到底部。但如果用户这时候**自己正用鼠标往上翻看前面的聊天记录**，就不能强行把它拽回底部。只有当用户的滚动条本来就在最底下附近时，才执行吸底滚动，体验才会好。”

---

### 🎤 面试题 3：既然后端已经拿到工具结果（比如查出了26℃晴天），为啥不直接丢给前端？非要再找大模型调一次？
> **面试官真实在问**：你到底明不明白 Agent 和传统接口调用的区别？

* **3 个词记忆小抄**：**说人话、答用户真正的问题、连环工具调用**
* **🗣️ 大白话回答**：
  > “如果直接把查出来的天气扔给前端，那跟我们以前写一个搜索框直接调 API 没什么区别，那就不是 AI 智能体了。
  > 
  > 必须要大模型做第二轮总结，主要是因为：**用户要的往往不是冷冰冰的数据，而是一句完整的解答。**
  > 比如用户问：‘我明天在杭州穿什么衣服合适？’ 工具查出来的只是‘26℃ 晴天’。如果直接把这个温度扔给用户就很生硬。把 26℃ 喂回给大模型，大模型就能根据温度组织人话：‘明天杭州 26℃ 晴朗，建议穿单件长袖或短袖加薄外套，非常适宜出门。’ 这样才是真正的智能助手闭环。”

---

## 三、动手实操四步法（Step-by-Step）

请打开文件：[frontend/src/views/chat/index.vue](file:///Volumes/ORICO/Desktop_backup/test/mini-agent/frontend/src/views/chat/index.vue)，按步骤自主实现：

---

### 步骤 1：扩展 TypeScript 消息数据模型

找到顶部的 `interface ChatMessage`（大约第 10~16 行），添加工具信息接口 `ToolInfo`：

```typescript
// 1. 定义工具状态数据接口
interface ToolInfo {
  name: string;
  args?: string;
  result?: string;
  status: "calling" | "done";
}

// 2. 在 ChatMessage 中增加 tool 可选属性
interface ChatMessage {
  id: string;
  role: "user" | "assistant" | "system";
  content: string;
  reasoning?: string;
  tool?: ToolInfo; // 👈 新增这一行
  timestamp: string;
}
```

---

### 步骤 2：在 SSE 循环中解析工具状态帧

找到 `handleSend` 方法中的 SSE 解析循环（大约第 125~138 行）：

```typescript
try {
  const parsed = JSON.parse(dataStr);
  if (parsed.error) {
    ElMessage.error(parsed.error);
  } else if (targetMsg) {
    // 1. 思考过程收集
    if (parsed.reasoning) {
      targetMsg.reasoning = (targetMsg.reasoning || "") + parsed.reasoning;
      scrollToBottom();
    }
    
    // 2. 👇 新增：工具调用状态解析
    if (parsed.tool_name) {
      targetMsg.tool = {
        name: parsed.tool_name,
        args: parsed.tool_args || targetMsg.tool?.args,
        result: parsed.tool_result || targetMsg.tool?.result,
        status: parsed.tool_status || "calling"
      };
      scrollToBottom();
    }

    // 3. 正文回答收集
    if (parsed.content) {
      targetMsg.content += parsed.content;
      scrollToBottom();
    }
  }
} catch {
  // 非 JSON 数据
}
```

---

### 步骤 3：在 Template 中新增工具交互卡片

找到 `<template>` 中的 `<div class="bubble-content">` 内部（大约第 220 行附近，在 `</details>` 之后、`<div class="markdown-body">` 之前），插入以下工具卡片模板：

```html
<!-- ⚙️ 外部工具调用状态卡片 -->
<div v-if="msg.tool" class="tool-call-box" :class="msg.tool.status">
  <div class="tool-call-header">
    <div class="tool-icon-wrap">
      <span v-if="msg.tool.status === 'calling'" class="tool-spin">⚙️</span>
      <span v-else class="tool-check">✅</span>
    </div>
    <div class="tool-title">
      <span class="tool-label">
        {{ msg.tool.status === 'calling' ? '智能体正在调用工具' : '已完成工具调用' }}:
      </span>
      <span class="tool-name">{{ msg.tool.name }}</span>
    </div>
  </div>
  
  <!-- 参数与执行结果折叠展示 -->
  <div class="tool-call-body">
    <div v-if="msg.tool.args" class="tool-item">
      <span class="item-label">提取入参:</span>
      <code class="item-code">{{ msg.tool.args }}</code>
    </div>
    <div v-if="msg.tool.result" class="tool-item">
      <span class="item-label">执行结果:</span>
      <code class="item-code result">{{ msg.tool.result }}</code>
    </div>
  </div>
</div>
```

---

### 步骤 4：添加卡片质感与微动效样式

在底部 `<style scoped>` 中追加以下样式（打造现代磨砂质感卡片与齿轮旋转动效）：

```css
/* 工具调用卡片容器 */
.tool-call-box {
  margin-bottom: 12px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 10px 14px;
  font-size: 13px;
  transition: all 0.3s ease;
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

.tool-call-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}

.tool-spin {
  display: inline-block;
  animation: spin 2s linear infinite;
  font-size: 15px;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.tool-label {
  color: #64748b;
  font-size: 12px;
}

.tool-name {
  color: #1e293b;
  font-family: monospace;
  background: rgba(0, 0, 0, 0.05);
  padding: 2px 6px;
  border-radius: 4px;
  margin-left: 4px;
}

.tool-call-body {
  margin-top: 8px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  border-top: 1px dashed rgba(0, 0, 0, 0.08);
  padding-top: 6px;
}

.tool-item {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  font-size: 12px;
}

.item-label {
  color: #94a3b8;
  flex-shrink: 0;
}

.item-code {
  font-family: Menlo, Monaco, Consolas, monospace;
  background: rgba(0, 0, 0, 0.04);
  padding: 2px 6px;
  border-radius: 4px;
  word-break: break-all;
  color: #334155;
}

.item-code.result {
  color: #166534;
}
```

---

## 四、最终效果验收与自测

完成上述修改后：
1. 浏览器打开本地前端页面：`http://localhost:8848/#/chat/index`（或你的 Vite 开发端口）；
2. 在输入框输入：**“帮我查一下杭州今天天气怎么样？”**；
3. **观察三段式交互**：
   - 🧠 先出现【深度思考】；
   - ⚙️ 接着弹出【正在调用工具：get_weather】齿轮旋转动画；
   - ✅ 工具执行完毕，变绿并显示查到的 JSON 结果；
   - 💬 最终流式打字输出自然语言总结！

照着这份文档改好后，随时在界面上测试，并在有疑问时随时与我探讨！
