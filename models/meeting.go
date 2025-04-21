package models

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/schema"
	"github.com/hertz-contrib/sse"
)

// Meeting represents a meeting entity
type Meeting struct {
	ID      string                 `json:"id"`
	Content map[string]interface{} `json:"content"`
}

// PostMeetingResponse represents the response for creating a meeting
type PostMeetingResponse struct {
	ID string `json:"id"`
}

// GetMeetingsResponse represents the response for listing meetings
type GetMeetingsResponse struct {
	Meetings []Meeting `json:"meetings"`
}

// ChatMessage represents a chat message in the SSE stream
type ChatMessage struct {
	Data string `json:"data"`
}

// RolePlayMessage 表示角色扮演聊天消息
type RolePlayMessage struct {
	Data            string `json:"data"`             // 会议内容数据
	ParticipantName string `json:"participant_name"` // 参会人姓名
}

// MeetingScore 表示会议评分结果
type MeetingScore struct {
	GoalAchievement       int     `json:"goal_achievement"`       // 会议目标达成度
	TopicFocus            int     `json:"topic_focus"`            // 主题聚焦度
	ParticipantEngagement int     `json:"participant_engagement"` // 参与者互动与参与度
	TotalScore            int     `json:"total_score"`            // 总分
	MaxPossibleScore      int     `json:"max_possible_score"`     // 最大可能得分
	ScorePercentage       float64 `json:"score_percentage"`       // 得分百分比
	Feedback              string  `json:"feedback"`               // 评价反馈
}

// FeiShuWebhookConfig 飞书机器人配置
type FeiShuWebhookConfig struct {
	WebhookURL string `json:"webhook_url"` // 飞书机器人的Webhook地址
}

// MeetingReport 表示会议报告
type MeetingReport struct {
	Title        string   `json:"title"`        // 会议标题
	Description  string   `json:"description"`  // 会议描述
	Summary      string   `json:"summary"`      // 会议摘要
	Participants []string `json:"participants"` // 参会人员
	TodoList     []string `json:"todo_list"`    // 待办事项
}

// FeiShuMessage 表示飞书消息的结构
type FeiShuMessage struct {
	MsgType string `json:"msg_type"`
	Card    Card   `json:"card"`
}

// Card 表示飞书卡片消息结构
type Card struct {
	Header   Header    `json:"header"`
	Elements []Element `json:"elements"`
}

// Header 表示飞书卡片的标题
type Header struct {
	Title    Title  `json:"title"`
	Template string `json:"template"` // blue, green, turquoise, red, orange, purple, grey
}

// Title 表示飞书卡片标题
type Title struct {
	Content string `json:"content"`
	Tag     string `json:"tag"`
}

// Element 表示飞书卡片中的元素
type Element struct {
	Tag     string   `json:"tag"`
	Text    *Text    `json:"text,omitempty"`
	Fields  []Field  `json:"fields,omitempty"`
	Actions []Action `json:"actions,omitempty"`
}

// Text 表示飞书卡片中的文本
type Text struct {
	Content string `json:"content"`
	Tag     string `json:"tag"`
}

// Field 表示飞书卡片中的字段
type Field struct {
	IsShort bool `json:"is_short"`
	Text    Text `json:"text"`
}

// Action 表示飞书卡片中的操作
type Action struct {
	Tag   string `json:"tag"`
	Text  Text   `json:"text"`
	URL   string `json:"url,omitempty"`
	Type  string `json:"type,omitempty"`
	Value string `json:"value,omitempty"`
}

func Of[T any](v T) *T {
	return &v
}

// Process handles the chat message and returns streaming response to the SSE stream
func (c ChatMessage) Process(query string, stream *sse.Stream) error {
	// 从配置文件中获取API密钥和模型名称
	arkAPIKey, err := GetARKAPIKey()
	if err != nil {
		fmt.Printf("获取API密钥失败: %v", err)
		event := &sse.Event{
			Data: []byte(fmt.Sprintf(`{"data":"%s"}`, "错误: 获取API密钥失败")),
		}
		return stream.Publish(event)
	}

	arkModelName, err := GetARKModelName()
	if err != nil {
		fmt.Printf("获取模型名称失败: %v", err)
		event := &sse.Event{
			Data: []byte(fmt.Sprintf(`{"data":"%s"}`, "错误: 获取模型名称失败")),
		}
		return stream.Publish(event)
	}

	ctx := context.Background()
	arkModel, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:      arkAPIKey,
		Model:       arkModelName,
		Temperature: Of(float32(0.6)),
	})
	if err != nil {
		fmt.Printf("failed to create chat model: %v", err)
		event := &sse.Event{
			Data: []byte(fmt.Sprintf(`{"data":"%s"}`, "错误: 创建聊天模型失败")),
		}
		return stream.Publish(event)
	}

	// 拼接c.Data和query作为prompt
	prompt := c.Data
	if query != "" {
		prompt = prompt + "\n用户问题: " + query
	}

	// 准备消息
	messages := []*schema.Message{
		schema.SystemMessage("你是一个会议助手，负责回答用户关于会议内容的问题。"),
		schema.UserMessage(prompt),
	}

	// 使用流式生成回答
	reader, err := arkModel.Stream(ctx, messages)
	if err != nil {
		fmt.Printf("failed to generate streaming response: %v", err)
		event := &sse.Event{
			Data: []byte(fmt.Sprintf(`{"data":"%s"}`, "错误: 生成流式回答失败")),
		}
		return stream.Publish(event)
	}
	defer reader.Close()

	// 处理流式响应
	var fullResponse strings.Builder
	for {
		chunk, err := reader.Recv()
		if err != nil {
			// 流结束或发生错误
			break
		}

		fullResponse.WriteString(chunk.Content)

		// 将每个块作为SSE事件发送
		jsonResponse := fmt.Sprintf(`{"data":%q}`, chunk.Content)
		event := &sse.Event{
			Data: []byte(jsonResponse),
		}

		if err := stream.Publish(event); err != nil {
			fmt.Printf("发送SSE事件失败: %v", err)
			return err
		}
	}

	return nil
}

// 原始非流式Process方法，保留作为参考或备用
func (c ChatMessage) ProcessNonStream(query string) string {
	// 从配置文件中获取API密钥和模型名称
	arkAPIKey, err := GetARKAPIKey()
	if err != nil {
		fmt.Printf("获取API密钥失败: %v", err)
		return "错误: 获取API密钥失败"
	}

	arkModelName, err := GetARKModelName()
	if err != nil {
		fmt.Printf("获取模型名称失败: %v", err)
		return "错误: 获取模型名称失败"
	}

	ctx := context.Background()
	arkModel, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:      arkAPIKey,
		Model:       arkModelName,
		Temperature: Of(float32(0.6)),
	})
	if err != nil {
		fmt.Printf("failed to create chat model: %v", err)
		return "错误: 创建聊天模型失败"
	}

	// 拼接c.Data和query作为prompt
	prompt := c.Data
	if query != "" {
		prompt = prompt + "\n用户问题: " + query
	}

	// 准备消息
	messages := []*schema.Message{
		schema.SystemMessage("你是一个会议助手，负责回答用户关于会议内容的问题。"),
		schema.UserMessage(prompt),
	}

	// 生成回答
	response, err := arkModel.Generate(ctx, messages)
	if err != nil {
		fmt.Printf("failed to generate response: %v", err)
		return "错误: 生成回答失败"
	}

	// 返回模型回答
	jsonResponse := fmt.Sprintf(`{"data":%q}`, response.Content)
	return jsonResponse
}

// ExtractMeetingInfo 使用LLM从会议文本中提取结构化信息
func ExtractMeetingInfo(ctx context.Context, documentText string) (map[string]interface{}, error) {
	// 从配置文件中获取API密钥和模型名称
	arkAPIKey, err := GetARKAPIKey()
	if err != nil {
		return nil, fmt.Errorf("获取API密钥失败: %v", err)
	}

	arkModelName, err := GetARKModelName()
	if err != nil {
		return nil, fmt.Errorf("获取模型名称失败: %v", err)
	}

	arkModel, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:      arkAPIKey,
		Model:       arkModelName,
		Temperature: Of(float32(0.8)), // 低温度以获得更确定性的结果
	})

	if err != nil {
		return nil, fmt.Errorf("创建LLM客户端失败: %v", err)
	}

	// 准备系统提示和用户提示
	systemPrompt := `你是一个专业的会议分析助手。请从会议文本中提取以下信息：
1. 会议标题(必须包含)
2. 会议描述或主题(必须包含)
3. 参会人员列表(必须包含)
4. 会议开始时间（尽可能精确到日期和时间）
5. 会议结束时间（尽可能精确到日期和时间）
6. 会议主要内容摘要(不超过100字)
7. 会议中提到的一些待办事项(必须包含)

以JSON格式返回,字段包括:title, description, participants(数组), start_time, end_time, summary, todo_list(数组)。`

	// 准备消息
	messages := []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(documentText),
	}

	// 生成回答
	response, err := arkModel.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("生成分析失败: %v", err)
	}

	// 解析JSON响应
	var meetingInfo map[string]interface{}
	if err := json.Unmarshal([]byte(response.Content), &meetingInfo); err != nil {
		// 如果解析失败，尝试从文本中提取JSON部分
		jsonStartIdx := strings.Index(response.Content, "{")
		jsonEndIdx := strings.LastIndex(response.Content, "}")

		if jsonStartIdx >= 0 && jsonEndIdx > jsonStartIdx {
			jsonText := response.Content[jsonStartIdx : jsonEndIdx+1]
			if err := json.Unmarshal([]byte(jsonText), &meetingInfo); err != nil {
				return nil, fmt.Errorf("解析会议信息失败: %v", err)
			}
		} else {
			// 如果无法提取JSON，则创建一个基本结构
			meetingInfo = map[string]interface{}{
				"title":       "未知会议",
				"description": "无法从文本中提取会议信息",
				"summary":     response.Content,
			}
		}
	}

	return meetingInfo, nil
}

// ExtractMermaid 使用LLM从会议文本中总结出会议流程并输出对应的mermaid代码
func ExtractMermaid(ctx context.Context, documentText string) (string, error) {
	// 从配置文件中获取API密钥和模型名称
	arkAPIKey, err := GetARKAPIKey()
	if err != nil {
		return "", fmt.Errorf("获取API密钥失败: %v", err)
	}

	arkModelName, err := GetARKModelName()
	if err != nil {
		return "", fmt.Errorf("获取模型名称失败: %v", err)
	}

	arkModel, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:      arkAPIKey,
		Model:       arkModelName,
		Temperature: Of(float32(0.7)), // 稍微提高创造性
	})

	if err != nil {
		return "", fmt.Errorf("创建LLM客户端失败: %v", err)
	}

	// 准备系统提示和用户提示
	systemPrompt := `你是一个专业的会议流程分析专家，精通mermaid流程图语法。请从会议文本中提取主要流程和决策过程：
1. 分析会议中讨论的主要流程、决策过程或任务安排
2. 使用mermaid流程图语法创建一个清晰的流程图
3. 流程图应该包含主要步骤、决策点和责任人（如果有）
4. 保持图表简洁但信息丰富，通常不超过10个节点
5. 必须使用合法有效的mermaid语法

请直接返回完整的mermaid代码块，格式如下：
'''mermaid
flowchart TD
    A[开始] --> B[步骤1]
    B --> C{决策点}
    C -->|是| D[步骤2]
    C -->|否| E[步骤3]
    ...
'''

除了上述mermaid代码块外，请勿输出任何其他内容。`

	// 准备消息
	messages := []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(documentText),
	}

	// 生成回答
	response, err := arkModel.Generate(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("生成流程图失败: %v", err)
	}

	// 提取mermaid代码块内容
	content := response.Content
	mermaidStart := strings.Index(content, "'''mermaid")
	mermaidEnd := strings.LastIndex(content, "'''")

	if mermaidStart >= 0 && mermaidEnd > mermaidStart {
		// 提取mermaid代码，包括'''mermaid和结束的'''
		mermaidCode := content[mermaidStart : mermaidEnd+3]
		return mermaidCode, nil
	}

	// 如果没有找到合适的格式，则返回完整内容
	return content, nil
}

// ProcessRolePlay 处理角色扮演聊天并返回流式响应
func (r RolePlayMessage) ProcessRolePlay(query string, stream *sse.Stream) error {
	// 从配置文件中获取API密钥和模型名称
	arkAPIKey, err := GetARKAPIKey()
	if err != nil {
		fmt.Printf("获取API密钥失败: %v", err)
		event := &sse.Event{
			Data: []byte(fmt.Sprintf(`{"data":"%s"}`, "错误: 获取API密钥失败")),
		}
		return stream.Publish(event)
	}

	arkModelName, err := GetARKModelName()
	if err != nil {
		fmt.Printf("获取模型名称失败: %v", err)
		event := &sse.Event{
			Data: []byte(fmt.Sprintf(`{"data":"%s"}`, "错误: 获取模型名称失败")),
		}
		return stream.Publish(event)
	}

	ctx := context.Background()
	arkModel, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:      arkAPIKey,
		Model:       arkModelName,
		Temperature: Of(float32(0.7)), // 增加一点创造性，使角色扮演更生动
	})
	if err != nil {
		fmt.Printf("failed to create chat model: %v", err)
		event := &sse.Event{
			Data: []byte(fmt.Sprintf(`{"data":"%s"}`, "错误: 创建聊天模型失败")),
		}
		return stream.Publish(event)
	}

	// 拼接角色扮演提示
	prompt := fmt.Sprintf(`
会议内容: 
%s

你现在扮演的角色是会议中的参会者"%s"。你需要以这个人的身份、语气、知识背景、态度和说话方式来回答问题。
根据会议记录中这个人的言论和表现来模仿他/她的回答风格。如果会议内容中没有足够的信息来确定这个人的某些观点，
你可以基于这个人在会议中表现出的性格特点来合理推测，但要保持一致性。

用户将以对话形式向你提问，你要始终保持角色扮演，不要暴露你是AI的事实。回答要简洁自然，符合真实对话的风格。

用户问题: %s
`, r.Data, r.ParticipantName, query)

	// 准备消息
	messages := []*schema.Message{
		schema.SystemMessage("你正在进行角色扮演，扮演会议参会者。请完全沉浸在角色中，使用第一人称回答问题，仿佛你就是那个人。"),
		schema.UserMessage(prompt),
	}

	// 使用流式生成回答
	reader, err := arkModel.Stream(ctx, messages)
	if err != nil {
		fmt.Printf("failed to generate streaming response: %v", err)
		event := &sse.Event{
			Data: []byte(fmt.Sprintf(`{"data":"%s"}`, "错误: 生成流式回答失败")),
		}
		return stream.Publish(event)
	}
	defer reader.Close()

	// 处理流式响应
	var fullResponse strings.Builder
	for {
		chunk, err := reader.Recv()
		if err != nil {
			// 流结束或发生错误
			break
		}

		fullResponse.WriteString(chunk.Content)

		// 将每个块作为SSE事件发送
		jsonResponse := fmt.Sprintf(`{"data":%q, "role":"%s"}`, chunk.Content, r.ParticipantName)
		event := &sse.Event{
			Data: []byte(jsonResponse),
		}

		if err := stream.Publish(event); err != nil {
			fmt.Printf("发送SSE事件失败: %v", err)
			return err
		}
	}

	return nil
}

// EvaluateMeeting 使用LLM评估会议质量
func EvaluateMeeting(ctx context.Context, documentText string) (*MeetingScore, error) {
	// 从配置文件中获取API密钥和模型名称
	arkAPIKey, err := GetARKAPIKey()
	if err != nil {
		return nil, fmt.Errorf("获取API密钥失败: %v", err)
	}

	arkModelName, err := GetARKModelName()
	if err != nil {
		return nil, fmt.Errorf("获取模型名称失败: %v", err)
	}

	arkModel, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:      arkAPIKey,
		Model:       arkModelName,
		Temperature: Of(float32(0.2)), // 低温度以获得一致的评估结果
	})

	if err != nil {
		return nil, fmt.Errorf("创建LLM客户端失败: %v", err)
	}

	// 准备系统提示和用户提示
	systemPrompt := `你是一个专业的会议评估专家。你需要根据以下评分规则对提供的会议文本进行全面客观的评估：

核心指标一：会议目标达成度 (Meeting Goal Achievement) - 总分 /4
4 分 (优秀): 会议完全实现了预定的目标，目标非常明确且可衡量，产出了清晰、可执行的成果和行动项，问题（如果会议目的是解决问题）得到了高效解决。
3 分 (良好): 会议基本实现了预定的目标，目标比较明确，产出了较为具体的成果和行动项，问题（如果会议目的是解决问题）得到了较好解决。
2 分 (一般): 会议部分实现了预定的目标，目标相对模糊，成果和行动项较为笼统，问题（如果会议目的是解决问题）得到了初步讨论，但解决程度有限。
1 分 (较差): 会议未能有效实现预定的目标，目标不明确，缺乏有效成果和行动项，问题（如果会议目的是解决问题）未得到有效解决。

核心指标二：主题聚焦度 (Topic Focus) - 总分 /4
4 分 (优秀): 讨论完全聚焦于会议主题和议程，严格遵循议程，所有内容高度相关，时间利用非常高效，无跑题。
3 分 (良好): 讨论基本聚焦于会议主题和议程，大部分遵循议程，内容基本相关，时间利用效率较高，偶有少量跑题但能及时拉回。
2 分 (一般): 讨论部分偏离会议主题和议程，议程遵循度一般，部分内容关联性较弱，时间利用效率一般，跑题现象较为明显。
1 分 (较差): 讨论严重偏离会议主题和议程，议程形同虚设，大量内容无关，时间利用效率极低，严重跑题。

核心指标三：参与者互动与参与度 (Participant Engagement & Interaction) - 总分 /4
4 分 (优秀): 绝大多数参与者都积极参与，互动频繁且深入，认真倾听并尊重他人，讨论氛围非常积极合作，充分体现集体智慧。
3 分 (良好): 多数参与者都积极参与，互动较好，基本认真倾听并尊重他人，讨论氛围较为友好，参与度良好。
2 分 (一般): 部分参与者参与，互动较少，倾听和尊重程度一般，讨论氛围有待改善，参与度一般，部分人沉默。
1 分 (较差): 少数人主导，参与度极低，几乎没有互动，缺乏倾听和尊重，讨论氛围紧张或冷淡，如同单向汇报。

必须严格按照以上评分标准，根据会议文本的内容和质量，为每个核心指标打分，并给出总体评价。你的评估必须客观、公正、详细，基于事实而非主观假设。
你的回答必须包含每个指标的得分（1-4分）和详细理由，以及一个总体评价。

以下是你必须返回的JSON格式（不要输出其他内容）：
{
  "goal_achievement": 分数,
  "goal_achievement_feedback": "理由...",
  "topic_focus": 分数,
  "topic_focus_feedback": "理由...",
  "participant_engagement": 分数,
  "participant_engagement_feedback": "理由...",
  "overall_feedback": "总体评价..."
}`

	// 准备消息
	messages := []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(documentText),
	}

	// 生成回答
	response, err := arkModel.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("评估会议失败: %v", err)
	}

	// 解析评估结果
	var evaluation map[string]interface{}
	if err := json.Unmarshal([]byte(response.Content), &evaluation); err != nil {
		// 如果解析失败，尝试从文本中提取JSON部分
		jsonStartIdx := strings.Index(response.Content, "{")
		jsonEndIdx := strings.LastIndex(response.Content, "}")

		if jsonStartIdx >= 0 && jsonEndIdx > jsonStartIdx {
			jsonText := response.Content[jsonStartIdx : jsonEndIdx+1]
			if err := json.Unmarshal([]byte(jsonText), &evaluation); err != nil {
				return nil, fmt.Errorf("解析评估结果失败: %v", err)
			}
		} else {
			return nil, fmt.Errorf("评估结果格式错误: %v", err)
		}
	}

	// 提取评分
	goalAchievement, _ := evaluation["goal_achievement"].(float64)
	topicFocus, _ := evaluation["topic_focus"].(float64)
	participantEngagement, _ := evaluation["participant_engagement"].(float64)

	// 计算总分和百分比
	totalScore := int(goalAchievement + topicFocus + participantEngagement)
	maxPossibleScore := 12 // 3个指标，每个最高4分
	scorePercentage := float64(totalScore) / float64(maxPossibleScore) * 100

	// 构建反馈
	goalAchievementFeedback, _ := evaluation["goal_achievement_feedback"].(string)
	topicFocusFeedback, _ := evaluation["topic_focus_feedback"].(string)
	participantEngagementFeedback, _ := evaluation["participant_engagement_feedback"].(string)
	overallFeedback, _ := evaluation["overall_feedback"].(string)

	feedback := fmt.Sprintf(`## 会议评分详情

### 会议目标达成度: %d/4
%s

### 主题聚焦度: %d/4
%s

### 参与者互动与参与度: %d/4
%s

### 总体评价
%s

**总分: %d/%d (%.1f%%)**
`,
		int(goalAchievement),
		goalAchievementFeedback,
		int(topicFocus),
		topicFocusFeedback,
		int(participantEngagement),
		participantEngagementFeedback,
		overallFeedback,
		totalScore,
		maxPossibleScore,
		scorePercentage)

	// 构建评分结果
	meetingScore := &MeetingScore{
		GoalAchievement:       int(goalAchievement),
		TopicFocus:            int(topicFocus),
		ParticipantEngagement: int(participantEngagement),
		TotalScore:            totalScore,
		MaxPossibleScore:      maxPossibleScore,
		ScorePercentage:       scorePercentage,
		Feedback:              feedback,
	}

	return meetingScore, nil
}

// GetFeiShuWebhookURL 从配置中获取飞书Webhook URL
func GetFeiShuWebhookURL() (string, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return "", err
	}

	// 从配置中读取飞书Webhook URL
	// 注意：需要在配置中添加feishu.webhook_url字段
	// 这里假设配置文件已添加该字段，如果没有，需要更新Config结构和config.json
	if cfg.FeiShu.WebhookURL == "" {
		return "", fmt.Errorf("飞书Webhook URL未配置")
	}

	return cfg.FeiShu.WebhookURL, nil
}

// CreateMeetingReport 从会议ID创建会议报告
func CreateMeetingReport(meetingID string) (*MeetingReport, error) {
	// 读取对应会议文件内容
	storageDir := "./storage/meetings"
	filePath := filepath.Join(storageDir, meetingID+".json")

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("会议不存在")
	}

	// 读取会议文件
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("无法读取会议信息: %v", err)
	}

	// 解析JSON内容
	var meetingData map[string]interface{}
	if err := json.Unmarshal(data, &meetingData); err != nil {
		return nil, fmt.Errorf("无法解析会议数据: %v", err)
	}

	// 创建会议报告
	report := &MeetingReport{
		Title:        "未命名会议",
		Description:  "",
		Summary:      "",
		Participants: []string{},
		TodoList:     []string{},
	}

	// 从metadata中提取信息
	if metadata, ok := meetingData["metadata"].(map[string]interface{}); ok {
		// 提取标题
		if title, ok := metadata["title"].(string); ok && title != "" {
			report.Title = title
		}

		// 提取描述
		if description, ok := metadata["description"].(string); ok && description != "" {
			report.Description = description
		}

		// 提取摘要
		if summary, ok := metadata["summary"].(string); ok && summary != "" {
			report.Summary = summary
		}

		// 提取参会人员
		if participants, ok := metadata["participants"].([]interface{}); ok && len(participants) > 0 {
			for _, p := range participants {
				if pStr, ok := p.(string); ok && pStr != "" {
					report.Participants = append(report.Participants, pStr)
				}
			}
		}

		// 提取待办事项
		if todoList, ok := metadata["todo_list"].([]interface{}); ok && len(todoList) > 0 {
			for _, todo := range todoList {
				if todoStr, ok := todo.(string); ok && todoStr != "" {
					report.TodoList = append(report.TodoList, todoStr)
				}
			}
		}
	}

	return report, nil
}

// SendMeetingReportToFeiShu 发送会议报告到飞书
func SendMeetingReportToFeiShu(report *MeetingReport) error {
	// 获取飞书Webhook URL
	webhookURL, err := GetFeiShuWebhookURL()
	if err != nil {
		return fmt.Errorf("获取飞书Webhook URL失败: %v", err)
	}

	// 构建飞书消息
	message := FeiShuMessage{
		MsgType: "interactive",
		Card: Card{
			Header: Header{
				Title: Title{
					Content: report.Title,
					Tag:     "plain_text",
				},
				Template: "blue", // 可以根据需要更改颜色
			},
			Elements: []Element{},
		},
	}

	// 添加会议描述
	if report.Description != "" {
		message.Card.Elements = append(message.Card.Elements, Element{
			Tag: "div",
			Text: &Text{
				Content: "**会议描述：**\n" + report.Description,
				Tag:     "lark_md",
			},
		})
	}

	// 添加会议摘要
	if report.Summary != "" {
		message.Card.Elements = append(message.Card.Elements, Element{
			Tag: "div",
			Text: &Text{
				Content: "**会议摘要：**\n" + report.Summary,
				Tag:     "lark_md",
			},
		})
	}

	// 添加分割线
	message.Card.Elements = append(message.Card.Elements, Element{
		Tag: "hr",
	})

	// 添加参会人员
	if len(report.Participants) > 0 {
		participantsText := "**参会人员：**\n" + strings.Join(report.Participants, "、")
		message.Card.Elements = append(message.Card.Elements, Element{
			Tag: "div",
			Text: &Text{
				Content: participantsText,
				Tag:     "lark_md",
			},
		})
	}

	// 添加待办事项
	if len(report.TodoList) > 0 {
		todoListText := "**待办事项：**\n"
		for i, todo := range report.TodoList {
			todoListText += fmt.Sprintf("%d. %s\n", i+1, todo)
		}
		message.Card.Elements = append(message.Card.Elements, Element{
			Tag: "div",
			Text: &Text{
				Content: todoListText,
				Tag:     "lark_md",
			},
		})
	}

	// 将消息转换为JSON
	messageJSON, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %v", err)
	}

	// 发送POST请求到飞书Webhook
	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(messageJSON))
	if err != nil {
		return fmt.Errorf("发送消息到飞书失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("飞书返回错误状态码: %d, 响应: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// PushMeetingReportToFeiShu 根据会议ID创建报告并推送到飞书
func PushMeetingReportToFeiShu(meetingID string) error {
	// 创建会议报告
	report, err := CreateMeetingReport(meetingID)
	if err != nil {
		return fmt.Errorf("创建会议报告失败: %v", err)
	}

	// 发送报告到飞书
	if err := SendMeetingReportToFeiShu(report); err != nil {
		return fmt.Errorf("发送报告到飞书失败: %v", err)
	}

	return nil
}
