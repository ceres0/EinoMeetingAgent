package models

import (
	"context"
	"encoding/json"
	"fmt"
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
