# Meeting API Documentation

本文档提供了 Meeting API 接口的详细信息及使用方法。

## API 接口

### 会议管理接口

#### 1. 创建会议
创建新的会议并返回会议 ID。

**接口:** `POST /meeting`

**请求体:**
```json
{
  "title": "团队周会",
  "description": "周团队同步会议",
  "participants": ["张三", "李四"]
}
```

**响应:**
```json
{
  "id": "meeting_20250421112041"
}
```

**Curl 示例:**
```bash
curl -X POST http://localhost:8888/meeting \
  -H "Content-Type: application/json" \
  -d '{
    "title": "团队周会",
    "description": "周团队同步会议",
    "participants": ["张三", "李四"]
  }'
```

#### 2. 获取会议列表
获取所有会议的列表。

**接口:** `GET /meeting`

**响应:**
```json
{
  "meetings": [
    {
      "id": "meeting_20250421112041",
      "content": {
        "title": "团队周会",
        "description": "周团队同步会议",
        "participants": ["张三", "李四"]
      }
    }
  ]
}
```

**Curl 示例:**
```bash
curl -X GET http://localhost:8888/meeting
```

#### 3. 获取会议摘要
获取指定会议的摘要。

**接口:** `GET /summary`

**查询参数:**
- `meeting_id` (必填): 会议 ID，例如 "meeting_20250421112041"

**响应:**
```json
{
  "summary": "会议讨论要点和结论..."
}
```

**Curl 示例:**
```bash
curl -X GET "http://localhost:8888/summary?meeting_id=meeting_20250421112041"
```

#### 4. 获取会议 Mermaid 图表
获取会议内容的 Mermaid 图表。

**接口:** `GET /mermaid`

**查询参数:**
- `meeting_id` (必填): 会议 ID，例如 "meeting_20250421112041"

**响应:**
```json
{
  "mermaid": "graph TD\nA[会议开始] --> B[讨论项目进度]\nB --> C[任务分配]\nC --> D[会议结束]"
}
```

**Curl 示例:**
```bash
curl -X GET "http://localhost:8888/mermaid?meeting_id=meeting_20250421112041"
```

#### 5. 获取会议评分
获取会议的质量评分。

**接口:** `GET /score`

**查询参数:**
- `meeting_id` (必填): 会议 ID，例如 "meeting_20250421153445"

**响应:**
```json
{
  "score": 85,
  "comment": "会议高效，但缺少明确的行动项"
}
```

**Curl 示例:**
```bash
curl -X GET "http://localhost:8888/score?meeting_id=meeting_20250421153445"
```

### 聊天接口

#### 1. 实时聊天
建立 SSE 连接获取实时聊天消息。

**接口:** `GET /chat`

**查询参数:**
- `meeting_id` (必填): 会议 ID，例如 "meeting_20250421112041"
- `session_id` (必填): 聊天会话 ID，例如 "session_1745210662862"
- `message` (必填): 发送的消息，例如 "本次会议有哪些任务"

**响应:**
服务器发送事件(SSE)流，消息格式如下：
```json
{
  "data": {
    "message": "从会议内容来看，本次会议分配了以下任务：1. 张三负责准备项目进度报告...",
    "timestamp": "2024-03-21T10:00:00Z",
    "sender": "AI助手"
  }
}
```

**Curl 示例:**
```bash
curl -X GET "http://localhost:8888/chat?meeting_id=meeting_20250421112041&session_id=session_1745210662862&message=本次会议有哪些任务"
```

#### 2. 角色扮演聊天
支持角色扮演模式的聊天功能。

**接口:** `GET /roleplay`

**查询参数:**
- `meeting_id` (必填): 会议 ID，例如 "meeting_20250421135423"
- `session_id` (必填): 聊天会话 ID，例如 "session_1745210662862"
- `participant` (必填): 扮演的参会者角色，例如 "李泽煊"
- `message` (必填): 发送的消息，例如 "你在会议中提出了什么问题?"

**响应:**
服务器发送事件(SSE)流，消息格式如下：
```json
{
  "data": {
    "message": "在会议中，我（李泽煊）提出了关于项目时间线的问题，我询问了是否可以延长测试阶段的时间...",
    "timestamp": "2024-03-21T10:00:00Z",
    "sender": "李泽煊"
  }
}
```

**Curl 示例:**
```bash
curl -X GET "http://localhost:8888/roleplay?meeting_id=meeting_20250421135423&session_id=session_1745210662862&participant=李泽煊&message=你在会议中提出了什么问题?"
```

#### 3. 多角色扮演会议
创建多角色参与的模拟会议。

**接口:** `POST /multi-roleplay`

**请求体:**
```json
{
  "meeting_id": "meeting_20250421153445",
  "host": "江峰",
  "specialists": [
    "汪国庆",
    "施宇轩",
    "王启祥"
  ],
  "rounds": 3,
  "topic": "研究生怎么活得更精彩？"
}
```

**响应:**
```json
{
  "id": "multi_roleplay_456def",
  "messages": [
    {
      "role": "江峰",
      "content": "今天我们讨论的话题是'研究生怎么活得更精彩？'，首先请汪国庆老师发表看法。",
      "timestamp": "2024-03-21T10:00:00Z"
    },
    {
      "role": "汪国庆",
      "content": "谢谢主持人。我认为研究生活需要平衡学术和生活，建立良好的时间管理习惯...",
      "timestamp": "2024-03-21T10:00:01Z"
    },
    // 更多消息...
  ],
  "summary": "本次讨论围绕研究生如何平衡学业和生活展开，专家们提出了时间管理、拓展社交圈、培养爱好等多方面建议..."
}
```

**Curl 示例:**
```bash
curl -X POST http://localhost:8888/multi-roleplay \
  -H "Content-Type: application/json" \
  -d '{
    "meeting_id": "meeting_20250421153445",
    "host": "江峰",
    "specialists": [
      "汪国庆",
      "施宇轩",
      "王启祥"
    ],
    "rounds": 3,
    "topic": "研究生怎么活得更精彩？"
  }'
```

### 待办事项接口

#### 1. 创建待办事项
创建新的待办事项。

**接口:** `POST /todo`

**请求体:**
```json
{
  "title": "准备演示文稿",
  "description": "为下周的演讲准备幻灯片",
  "status": "未开始",
  "priority": 1,
  "due_date": "2023-05-10T14:00:00Z",
  "meeting_id": "meeting123",
  "assigned_to": "果松"
}
```

**响应:**
```json
{
  "id": 21,
  "title": "准备演示文稿",
  "status": "未开始"
}
```

**Curl 示例:**
```bash
curl -X POST http://localhost:8888/todo \
  -H "Content-Type: application/json" \
  -d '{
    "title": "准备演示文稿",
    "description": "为下周的演讲准备幻灯片",
    "status": "未开始",
    "priority": 1,
    "due_date": "2023-05-10T14:00:00Z",
    "meeting_id": "meeting123",
    "assigned_to": "果松"
  }'
```

#### 2. 获取待办事项列表
获取待办事项的列表。

**接口:** `GET /todo`

**查询参数:**
- `meeting_id` (可选): 筛选指定会议的待办事项，例如 "meeting123"
- `status` (可选): 筛选特定状态的待办事项，例如 "未开始"、"进行中"、"已完成"
- `priority` (可选): 筛选特定优先级的待办事项，例如 "1"

**响应:**
```json
{
  "todos": [
    {
      "id": 21,
      "title": "准备演示文稿",
      "description": "为下周的演讲准备幻灯片",
      "status": "未开始",
      "priority": 1,
      "due_date": "2023-05-10T14:00:00Z",
      "meeting_id": "meeting123",
      "assigned_to": "果松",
      "created_at": "2024-03-21T10:00:00Z"
    }
  ]
}
```

**Curl 示例:**
```bash
curl -X GET "http://localhost:8888/todo?meeting_id=meeting123&status=未开始&priority=1"
```

#### 3. 更新待办事项
更新指定 ID 的待办事项。

**接口:** `PUT /todo/:id`

**URL 参数:**
- `id` (必填): 待办事项 ID，例如 "21"

**请求体:**
```json
{
  "status": "进行中",
  "priority": 2
}
```

**响应:**
```json
{
  "id": 21,
  "title": "准备演示文稿",
  "status": "进行中",
  "updated_at": "2024-03-22T14:30:00Z"
}
```

**Curl 示例:**
```bash
curl -X PUT http://localhost:8888/todo/21 \
  -H "Content-Type: application/json" \
  -d '{
    "status": "进行中",
    "priority": 2
  }'
```

#### 4. 删除待办事项
删除指定 ID 的待办事项。

**接口:** `DELETE /todo/:id`

**URL 参数:**
- `id` (必填): 待办事项 ID，例如 "3"

**响应:**
```json
{
  "success": true,
  "message": "待办事项已删除"
}
```

**Curl 示例:**
```bash
curl -X DELETE http://localhost:8888/todo/3
```

### 报告接口

#### 1. 推送会议报告
推送会议报告到飞书。

**接口:** `GET /push-report`

**查询参数:**
- `meeting_id` (必填): 会议 ID，例如 "meeting_20250421112041"

**响应:**
```json
{
  "success": true,
  "message": "报告已成功推送到飞书",
  "meeting_id": "meeting_20250421112041"
}
```

**Curl 示例:**
```bash
curl -X GET "http://localhost:8888/push-report?meeting_id=meeting_20250421112041"
```

## 内容类型

- 所有常规接口使用 `application/json` 作为请求和响应体的内容类型
- 聊天和流式接口使用 `text/event-stream` 作为服务器发送事件流的内容类型 