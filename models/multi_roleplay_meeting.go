package models

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/schema"
	"github.com/hertz-contrib/sse"
)

// MultiRoleplayRequest 多角色扮演会议请求
type MultiRoleplayRequest struct {
	MeetingID   string   `json:"meeting_id"`
	Host        string   `json:"host"`
	Specialists []string `json:"specialists"`
	Rounds      int      `json:"rounds"`
	Topic       string   `json:"topic"`
}

// DiscussionMessage 讨论消息
type DiscussionMessage struct {
	Role     string `json:"role"`
	Content  string `json:"content"`
	IsSystem bool   `json:"is_system"`
}

// MultiRoleplayResponse 多角色扮演会议响应
type MultiRoleplayResponse struct {
	Messages []DiscussionMessage `json:"messages"`
	Summary  string              `json:"summary"`
}

// LogCallbackHandler 记录agent消息的处理器
type LogCallbackHandler struct {
	Messages     []DiscussionMessage
	messagesLock sync.Mutex
	Stream       *sse.Stream
	AgentNameMap map[string]string
}

// OnAgentMessage 处理Agent消息回调
func (h *LogCallbackHandler) OnAgentMessage(_ context.Context, msg *schema.Message) error {
	content := msg.Content

	h.messagesLock.Lock()
	defer h.messagesLock.Unlock()

	// 获取角色实际名称
	roleName := string(msg.Role)
	if actualName, exists := h.AgentNameMap[roleName]; exists && msg.Role != schema.System {
		roleName = actualName
	}

	// 添加消息到列表
	message := DiscussionMessage{
		Role:     roleName,
		Content:  content,
		IsSystem: msg.Role == schema.System,
	}
	h.Messages = append(h.Messages, message)

	// 发送SSE事件
	if h.Stream != nil {
		jsonData, err := json.Marshal(message)
		if err != nil {
			return err
		}

		event := &sse.Event{
			Data: jsonData,
		}

		if err := h.Stream.Publish(event); err != nil {
			return err
		}
	}

	return nil
}

// OnAgentHandoff 处理Agent切换回调
func (h *LogCallbackHandler) OnAgentHandoff(_ context.Context, reason string, targetAgent string) error {
	message := DiscussionMessage{
		Role:     "系统",
		Content:  fmt.Sprintf("【%s 将继续发言】", targetAgent),
		IsSystem: true,
	}

	h.messagesLock.Lock()
	defer h.messagesLock.Unlock()
	h.Messages = append(h.Messages, message)

	// 发送SSE事件
	if h.Stream != nil {
		jsonData, err := json.Marshal(message)
		if err != nil {
			return err
		}

		event := &sse.Event{
			Data: jsonData,
		}

		if err := h.Stream.Publish(event); err != nil {
			return err
		}
	}

	return nil
}

// Host 主持人代理
type Host struct {
	ChatModel    *ark.ChatModel
	SystemPrompt string
	Name         string
}

// Specialist 专家代理
type Specialist struct {
	Name         string
	ChatModel    *ark.ChatModel
	SystemPrompt string
}

// MultiAgent 多代理系统
type MultiAgent struct {
	Host        Host
	Specialists []Specialist
}

// NewMultiAgent 创建新的多代理系统
func NewMultiAgent(host Host, specialists []Specialist) *MultiAgent {
	return &MultiAgent{
		Host:        host,
		Specialists: specialists,
	}
}

// Stream 流式返回多代理系统的回答
func (ma *MultiAgent) Stream(ctx context.Context, messages []*schema.Message, cb *LogCallbackHandler) (io.ReadCloser, error) {
	// 创建管道用于流式返回
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		// 主持人发言
		hostMessages := append([]*schema.Message{
			schema.SystemMessage(ma.Host.SystemPrompt),
		}, messages...)

		hostResp, err := ma.Host.ChatModel.Generate(ctx, hostMessages)
		if err != nil {
			fmt.Fprintf(pw, "错误: %v", err)
			return
		}

		// 创建主持人消息
		hostMsg := &schema.Message{
			Role:    schema.Assistant,
			Content: hostResp.Content,
		}

		// 设置角色映射
		cb.AgentNameMap[string(schema.Assistant)] = ma.Host.Name

		// 记录主持人消息
		cb.OnAgentMessage(ctx, hostMsg)

		// 更新消息列表
		currentContext := append(messages, hostMsg)

		// 专家依次发言
		for _, specialist := range ma.Specialists {
			// 通知切换到专家
			cb.OnAgentHandoff(ctx, "轮到专家发言", specialist.Name)

			// 专家提示
			specialistPrompt := fmt.Sprintf("主持人%s邀请你(%s)发表意见。请根据主持人的提问，分享你的看法。",
				ma.Host.Name, specialist.Name)

			// 创建专家消息上下文
			specialistMessages := []*schema.Message{
				schema.SystemMessage(specialist.SystemPrompt),
			}

			// 添加对话历史
			specialistMessages = append(specialistMessages, currentContext...)

			// 添加点名提示
			specialistMessages = append(specialistMessages,
				schema.UserMessage(specialistPrompt))

			// 设置当前专家角色映射
			cb.AgentNameMap[string(schema.Assistant)] = specialist.Name

			// 生成专家回复
			specialistResp, err := specialist.ChatModel.Generate(ctx, specialistMessages)
			if err != nil {
				errMsg := fmt.Sprintf("专家%s回复失败: %v", specialist.Name, err)
				fmt.Fprintf(pw, errMsg)

				specialistMsg := &schema.Message{
					Role:    schema.Assistant,
					Content: "（因技术原因，暂未收到回复）",
				}

				cb.OnAgentMessage(ctx, specialistMsg)
				continue
			}

			// 确保回复不为空
			content := specialistResp.Content
			if strings.TrimSpace(content) == "" {
				content = fmt.Sprintf("（%s表示暂时没有补充意见）", specialist.Name)
			}

			// 创建专家消息
			specialistMsg := &schema.Message{
				Role:    schema.Assistant,
				Content: content,
			}

			// 记录专家消息
			cb.OnAgentMessage(ctx, specialistMsg)

			// 添加到当前上下文
			currentContext = append(currentContext,
				&schema.Message{
					Role:    schema.User,
					Content: fmt.Sprintf("%s: %s", specialist.Name, content),
				})
		}
	}()

	return pr, nil
}

// ProcessMultiRoleplayMeeting 处理多角色扮演会议
func ProcessMultiRoleplayMeeting(ctx context.Context, req *MultiRoleplayRequest, stream *sse.Stream) (*MultiRoleplayResponse, error) {
	// 获取会议内容
	meetingContent, meetingInfo, err := getMeetingContent(req.MeetingID)
	if err != nil {
		return nil, err
	}

	// 创建日志回调
	cb := &LogCallbackHandler{
		Messages:     []DiscussionMessage{},
		Stream:       stream,
		AgentNameMap: make(map[string]string),
	}

	// 会议开始系统消息
	startMsg := DiscussionMessage{
		Role:     "系统",
		Content:  "【会议扩展讨论开始】",
		IsSystem: true,
	}
	cb.Messages = append(cb.Messages, startMsg)

	if stream != nil {
		jsonData, _ := json.Marshal(startMsg)
		event := &sse.Event{
			Data: jsonData,
		}
		stream.Publish(event)
	}

	// 创建主持人代理
	hostAgent, err := newHost(ctx, req.Host, meetingContent, meetingInfo, req.Specialists)
	if err != nil {
		return nil, fmt.Errorf("创建主持人代理失败: %v", err)
	}

	// 创建专家代理
	specialists := make([]Specialist, 0, len(req.Specialists))
	for _, name := range req.Specialists {
		specialist, err := newSpecialist(ctx, name, meetingContent, meetingInfo, req.Host)
		if err != nil {
			return nil, fmt.Errorf("创建专家代理 %s 失败: %v", name, err)
		}
		specialists = append(specialists, specialist)
	}

	// 创建多代理
	multiAgent := NewMultiAgent(*hostAgent, specialists)

	// 讨论历史
	discussionHistory := []*schema.Message{}

	// 进行指定轮数对话
	for round := 0; round < req.Rounds; round++ {
		// 构建主持人指导消息
		var hostPrompt string
		if round == 0 {
			// 第一轮
			specialistsNames := strings.Join(req.Specialists, "、")
			if req.Topic != "" {
				hostPrompt = fmt.Sprintf("作为会议主持人，现在请你引导参会者们讨论以下主题：%s。在你的发言中，必须逐个点名邀请每位参会者（%s）发表意见。你的发言应该自然、富有引导性，并确保所有人都能参与讨论。", req.Topic, specialistsNames)
			} else {
				hostPrompt = fmt.Sprintf("作为会议主持人，请引导参会者们深入讨论会议中的重要议题。在你的发言中，必须逐个点名邀请每位参会者（%s）发表意见。你的发言应该自然、富有引导性，确保所有人都能参与讨论。", specialistsNames)
			}
		} else {
			// 后续轮次
			specialistsNames := strings.Join(req.Specialists, "、")
			hostPrompt = fmt.Sprintf("作为会议主持人，请对当前讨论进行简短总结，并继续引导讨论。在你的发言中，必须点名邀请每位参会者（%s）对讨论主题发表进一步的看法。确保所有人都能充分参与讨论，特别是那些之前发言不多的人。", specialistsNames)
		}

		// 构建本轮消息
		roundMessages := []*schema.Message{
			schema.SystemMessage(fmt.Sprintf("你是会议主持人%s。你的角色是引导讨论并确保每位参会者都有发言机会。你必须在发言中明确点名每位参会者，请他们发表意见。", req.Host)),
			schema.UserMessage(hostPrompt),
		}

		// 添加讨论历史
		roundMessages = append(roundMessages, discussionHistory...)

		// 流式生成回答
		out, err := multiAgent.Stream(ctx, roundMessages, cb)
		if err != nil {
			return nil, fmt.Errorf("第%d轮对话生成失败: %v", round+1, err)
		}

		io.Copy(io.Discard, out)
		out.Close()

		if round == req.Rounds-1 {
			break
		}

		// 收集本轮消息
		discussionHistory = collectRoundMessages(cb.Messages, req.Host, req.Specialists)
	}

	// 生成总结
	summary, err := generateDiscussionSummary(ctx, cb.Messages, meetingInfo)
	if err != nil {
		return nil, fmt.Errorf("生成讨论总结失败: %v", err)
	}

	// 添加总结消息
	summaryMsg := DiscussionMessage{
		Role:     "系统",
		Content:  fmt.Sprintf("【讨论总结】\n%s", summary),
		IsSystem: true,
	}
	cb.Messages = append(cb.Messages, summaryMsg)

	if stream != nil {
		jsonData, _ := json.Marshal(summaryMsg)
		event := &sse.Event{
			Data: jsonData,
		}
		stream.Publish(event)
	}

	return &MultiRoleplayResponse{
		Messages: cb.Messages,
		Summary:  summary,
	}, nil
}

// collectRoundMessages 收集本轮消息，转换为下一轮上下文
func collectRoundMessages(messages []DiscussionMessage, hostName string, specialistNames []string) []*schema.Message {
	var result []*schema.Message

	// 计算当前轮次开始位置
	startPos := 0
	for i, msg := range messages {
		if msg.IsSystem {
			startPos = i + 1
		}
	}

	// 收集非系统消息
	for i := startPos; i < len(messages); i++ {
		msg := messages[i]
		if !msg.IsSystem {
			var role schema.RoleType
			if msg.Role == hostName {
				role = schema.Assistant
			} else {
				role = schema.User
			}

			result = append(result, &schema.Message{
				Role:    role,
				Content: msg.Content,
			})
		}
	}

	return result
}

// getMeetingContent 获取会议内容和元数据
func getMeetingContent(meetingID string) (string, string, error) {
	// 读取会议文件
	storageDir := "./storage/meetings"
	filePath := filepath.Join(storageDir, meetingID+".json")

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", "", fmt.Errorf("会议不存在")
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", "", fmt.Errorf("无法读取会议信息: %v", err)
	}

	// 解析JSON
	var meetingData map[string]interface{}
	if err := json.Unmarshal(data, &meetingData); err != nil {
		return "", "", fmt.Errorf("无法解析会议数据: %v", err)
	}

	// 提取会议内容
	var meetingContent string

	if rawContent, ok := meetingData["raw_content"].(string); ok {
		meetingContent = rawContent
	} else if content, ok := meetingData["content"].(string); ok {
		meetingContent = content
	} else {
		contentBytes, _ := json.MarshalIndent(meetingData, "", "  ")
		meetingContent = string(contentBytes)
	}

	// 提取会议元数据
	meetingInfo := "会议信息:\n"

	if metadata, ok := meetingData["metadata"].(map[string]interface{}); ok {
		if title, ok := metadata["title"].(string); ok && title != "" {
			meetingInfo += "标题: " + title + "\n"
		}

		if description, ok := metadata["description"].(string); ok && description != "" {
			meetingInfo += "描述: " + description + "\n"
		}

		if participants, ok := metadata["participants"].([]interface{}); ok && len(participants) > 0 {
			meetingInfo += "参会人员: "
			for i, p := range participants {
				if i > 0 {
					meetingInfo += ", "
				}
				if pStr, ok := p.(string); ok {
					meetingInfo += pStr
				}
			}
			meetingInfo += "\n"
		}

		if startTime, ok := metadata["start_time"].(string); ok && startTime != "" {
			meetingInfo += "开始时间: " + startTime + "\n"
		}
		if endTime, ok := metadata["end_time"].(string); ok && endTime != "" {
			meetingInfo += "结束时间: " + endTime + "\n"
		}

		if summary, ok := metadata["summary"].(string); ok && summary != "" {
			meetingInfo += "摘要: " + summary + "\n"
		}
	}

	return meetingContent, meetingInfo, nil
}

// newHost 创建主持人代理
func newHost(ctx context.Context, hostName string, meetingContent string, meetingInfo string, specialists []string) (*Host, error) {
	// 获取API配置
	arkAPIKey, err := GetARKAPIKey()
	if err != nil {
		return nil, fmt.Errorf("获取API密钥失败: %v", err)
	}

	arkModelName, err := GetARKModelName()
	if err != nil {
		return nil, fmt.Errorf("获取模型名称失败: %v", err)
	}

	// 创建聊天模型
	chatModel, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:      arkAPIKey,
		Model:       arkModelName,
		Temperature: Of(float32(0.7)),
	})
	if err != nil {
		return nil, fmt.Errorf("创建聊天模型失败: %v", err)
	}

	// 参会者列表
	participantsStr := strings.Join(specialists, "、")

	// 系统提示
	systemPrompt := fmt.Sprintf(`你是会议主持人%s，负责引导和管理会议讨论。

会议背景信息:
%s

会议内容:
%s

参会人员：%s

作为主持人，你必须：
1. 在每次发言中，明确点名邀请每位参会者发表意见，不能遗漏任何一位参会者
2. 引导讨论朝有建设性的方向发展，确保讨论不偏离主题
3. 尊重每个参会者的意见，适当总结讨论内容，推动讨论深入
4. 确保每次发言都简洁、清晰，言语专业有礼貌
5. 以第一人称回应，不要暴露你是AI的事实

注意：你必须在每次发言中，明确提及并邀请所有参会者（%s）各自发表意见。这是你的首要任务。`,
		hostName, meetingInfo, meetingContent, participantsStr, participantsStr)

	return &Host{
		ChatModel:    chatModel,
		SystemPrompt: systemPrompt,
		Name:         hostName,
	}, nil
}

// newSpecialist 创建专家参会者代理
func newSpecialist(ctx context.Context, specialistName string, meetingContent string, meetingInfo string, hostName string) (Specialist, error) {
	// 获取API配置
	arkAPIKey, err := GetARKAPIKey()
	if err != nil {
		return Specialist{}, fmt.Errorf("获取API密钥失败: %v", err)
	}

	arkModelName, err := GetARKModelName()
	if err != nil {
		return Specialist{}, fmt.Errorf("获取模型名称失败: %v", err)
	}

	// 创建代理系统提示
	systemPrompt := fmt.Sprintf(`你是会议参会者%s，在会议中扮演你自己的角色。

会议背景信息:
%s

会议内容:
%s

当主持人%s或其他参会者向你提问或点名你发言时，你必须做出回应。

作为参会者%s，你应该:
1. 基于会议记录中你的言论和表现，保持一致的性格、语气和专业知识
2. 如果会议内容中提到了你的职位、专长或责任，请在发言中体现出来
3. 对主持人和其他参会者的提问或建议作出回应
4. 表达你自己的观点，可以适当提出建设性的意见或批评
5. 不要重复已经说过的内容，要推动讨论向前发展
6. 你的回复应简洁、清晰，言语专业有礼貌

请记住，当主持人点名邀请你发言时，你必须积极回应。以第一人称回应，不要暴露你是AI的事实。`,
		specialistName, meetingInfo, meetingContent, hostName, specialistName)

	// 创建聊天模型
	chatModel, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:      arkAPIKey,
		Model:       arkModelName,
		Temperature: Of(float32(0.7)),
	})
	if err != nil {
		return Specialist{}, fmt.Errorf("创建聊天模型失败: %v", err)
	}

	return Specialist{
		Name:         specialistName,
		ChatModel:    chatModel,
		SystemPrompt: systemPrompt,
	}, nil
}

// generateDiscussionSummary 生成讨论总结
func generateDiscussionSummary(ctx context.Context, messages []DiscussionMessage, meetingInfo string) (string, error) {
	// 获取API配置
	arkAPIKey, err := GetARKAPIKey()
	if err != nil {
		return "", fmt.Errorf("获取API密钥失败: %v", err)
	}

	arkModelName, err := GetARKModelName()
	if err != nil {
		return "", fmt.Errorf("获取模型名称失败: %v", err)
	}

	// 创建聊天模型
	chatModel, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:      arkAPIKey,
		Model:       arkModelName,
		Temperature: Of(float32(0.4)),
	})
	if err != nil {
		return "", fmt.Errorf("创建聊天模型失败: %v", err)
	}

	// 提取讨论内容
	var discussionContent strings.Builder
	discussionContent.WriteString("会议背景信息:\n")
	discussionContent.WriteString(meetingInfo)
	discussionContent.WriteString("\n\n讨论记录:\n")

	for _, msg := range messages {
		if !msg.IsSystem {
			discussionContent.WriteString(fmt.Sprintf("%s: %s\n\n", msg.Role, msg.Content))
		}
	}

	// 系统提示
	systemPrompt := `作为专业会议纪要专家，请对提供的会议讨论内容进行总结。总结应包括：
1. 讨论的主要话题和议题
2. 各方观点的概述
3. 达成的共识或结论
4. 需要进一步讨论的问题
5. 确定的下一步行动项目

总结应该清晰、简洁、客观，长度控制在300-500字之间。请以第三人称编写，不要添加个人评价。`

	// 准备消息
	promptMessages := []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(discussionContent.String()),
	}

	// 生成回答
	response, err := chatModel.Generate(ctx, promptMessages)
	if err != nil {
		return "", fmt.Errorf("生成总结失败: %v", err)
	}

	return response.Content, nil
}

// PerformMultiRoleplayMeeting 执行多角色扮演会议并返回结果
func PerformMultiRoleplayMeeting(req *MultiRoleplayRequest) (*MultiRoleplayResponse, error) {
	ctx := context.Background()
	return ProcessMultiRoleplayMeeting(ctx, req, nil)
}

// StreamMultiRoleplayMeeting 执行多角色扮演会议并流式返回结果
func StreamMultiRoleplayMeeting(ctx context.Context, req *MultiRoleplayRequest, stream *sse.Stream) error {
	_, err := ProcessMultiRoleplayMeeting(ctx, req, stream)
	return err
}
