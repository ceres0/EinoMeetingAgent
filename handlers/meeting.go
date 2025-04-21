package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"meetingagent/models"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/hertz-contrib/sse"
)

// CreateMeeting handles the creation of a new meeting
func CreateMeeting(ctx context.Context, c *app.RequestContext) {
	var reqBody map[string]interface{}
	if err := c.BindJSON(&reqBody); err != nil {
		c.JSON(consts.StatusBadRequest, utils.H{"error": err.Error()})
		return
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		c.JSON(consts.StatusBadRequest, utils.H{"error": err.Error()})
		return
	}

	fmt.Printf("create meeting: %s\n", string(jsonBody))

	// 生成会议ID
	meetingID := "meeting_" + time.Now().Format("20060102150405")

	// 创建存储目录
	storageDir := "./storage/meetings"
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		c.JSON(consts.StatusInternalServerError, utils.H{"error": "无法创建存储目录"})
		return
	}

	// 从原始文档中提取文本内容
	documentText := ""
	if content, ok := reqBody["content"].(string); ok {
		documentText = content
	} else {
		// 如果内容不是字符串，尝试转换整个请求体为字符串
		contentBytes, err := json.Marshal(reqBody)
		if err == nil {
			documentText = string(contentBytes)
		}
	}

	// 调用LLM抽取会议信息
	meetingInfo, err := models.ExtractMeetingInfo(ctx, documentText)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, utils.H{"error": "无法分析会议内容: " + err.Error()})
		return
	}

	// 构建完整的会议内容
	meetingData := map[string]interface{}{
		"metadata":    meetingInfo,
		"raw_content": documentText,
	}

	// 将处理后的会议数据序列化为JSON
	processedJSON, err := json.Marshal(meetingData)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, utils.H{"error": "无法序列化会议数据"})
		return
	}

	// 将JSON内容写入文件
	filePath := filepath.Join(storageDir, meetingID+".json")
	if err := os.WriteFile(filePath, processedJSON, 0644); err != nil {
		c.JSON(consts.StatusInternalServerError, utils.H{"error": "无法保存会议文档"})
		return
	}

	// 返回响应
	response := models.PostMeetingResponse{
		ID: meetingID,
	}

	c.JSON(consts.StatusOK, response)
}

// ListMeetings handles listing all meetings
func ListMeetings(ctx context.Context, c *app.RequestContext) {
	storageDir := "./storage/meetings"

	// 读取目录中的所有文件
	files, err := os.ReadDir(storageDir)
	if err != nil {
		if os.IsNotExist(err) {
			// 如果目录不存在，返回空列表
			c.JSON(consts.StatusOK, models.GetMeetingsResponse{
				Meetings: []models.Meeting{},
			})
			return
		}
		// 其他错误返回500
		c.JSON(consts.StatusInternalServerError, utils.H{"error": "无法读取会议列表"})
		return
	}

	// 存储所有会议的切片
	var meetings []models.Meeting

	// 遍历所有文件
	for _, file := range files {
		// 跳过目录和非json文件
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		// 读取文件内容
		filePath := filepath.Join(storageDir, file.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			// 记录错误但继续处理其他文件
			fmt.Printf("读取文件 %s 失败: %v\n", filePath, err)
			continue
		}

		// 解析JSON内容
		var meetingData map[string]interface{}
		if err := json.Unmarshal(data, &meetingData); err != nil {
			fmt.Printf("解析文件 %s 失败: %v\n", filePath, err)
			continue
		}

		// 从文件名中提取ID (去掉.json后缀)
		meetingID := strings.TrimSuffix(file.Name(), ".json")

		// 获取元数据信息
		var content map[string]interface{}

		if metadata, ok := meetingData["metadata"].(map[string]interface{}); ok {
			// 使用LLM提取的元数据
			content = metadata
			// 确保原始内容也包含在内
			if rawContent, ok := meetingData["raw_content"].(string); ok {
				content["content"] = rawContent
			}
		} else {
			// 兼容旧格式，或者使用整个数据
			content = meetingData
		}

		// 创建Meeting对象并添加到列表
		meeting := models.Meeting{
			ID:      meetingID,
			Content: content,
		}
		meetings = append(meetings, meeting)
	}

	// 返回所有会议
	response := models.GetMeetingsResponse{
		Meetings: meetings,
	}

	c.JSON(consts.StatusOK, response)
}

// GetMeetingSummary handles retrieving a meeting summary
func GetMeetingSummary(ctx context.Context, c *app.RequestContext) {
	meetingID := c.Query("meeting_id")
	if meetingID == "" {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "meeting_id is required"})
		return
	}
	fmt.Printf("meetingID: %s\n", meetingID)

	// TODO: Implement actual summary retrieval logic
	response := map[string]interface{}{
		"content": `
		Meeting summary for ` + meetingID + `## Summary
we talked about the project and the next steps, we will have a call next week to discuss the project in more detail.

......
		`,
	}

	c.JSON(consts.StatusOK, response)
}

// HandleChat handles the SSE chat session
func HandleChat(ctx context.Context, c *app.RequestContext) {
	meetingID := c.Query("meeting_id")
	sessionID := c.Query("session_id")
	message := c.Query("message")

	if meetingID == "" || sessionID == "" {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "meeting_id and session_id are required"})
		return
	}

	if message == "" {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "message is required"})
		return
	}

	fmt.Printf("meetingID: %s, sessionID: %s, message: %s\n", meetingID, sessionID, message)

	// 读取对应会议文件内容
	storageDir := "./storage/meetings"
	filePath := filepath.Join(storageDir, meetingID+".json")

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(consts.StatusNotFound, utils.H{"error": "会议不存在"})
		return
	}

	// 读取会议文件
	data, err := os.ReadFile(filePath)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, utils.H{"error": "无法读取会议信息"})
		return
	}

	// 解析JSON内容
	var meetingData map[string]interface{}
	if err := json.Unmarshal(data, &meetingData); err != nil {
		c.JSON(consts.StatusInternalServerError, utils.H{"error": "无法解析会议数据"})
		return
	}

	// 提取会议内容
	var meetingContent string

	// 尝试从新格式中获取原始内容
	if rawContent, ok := meetingData["raw_content"].(string); ok {
		meetingContent = rawContent
	} else {
		// 尝试获取content字段
		if content, ok := meetingData["content"].(string); ok {
			meetingContent = content
		} else {
			// 如果没有找到适合的字段，将整个JSON作为内容
			contentBytes, _ := json.MarshalIndent(meetingData, "", "  ")
			meetingContent = string(contentBytes)
		}
	}

	// 提取会议元数据
	meetingInfo := "会议信息:\n"

	// 尝试从新格式中获取元数据
	if metadata, ok := meetingData["metadata"].(map[string]interface{}); ok {
		// 添加标题
		if title, ok := metadata["title"].(string); ok && title != "" {
			meetingInfo += "标题: " + title + "\n"
		}

		// 添加描述
		if description, ok := metadata["description"].(string); ok && description != "" {
			meetingInfo += "描述: " + description + "\n"
		}

		// 添加参会人员
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

		// 添加时间信息
		if startTime, ok := metadata["start_time"].(string); ok && startTime != "" {
			meetingInfo += "开始时间: " + startTime + "\n"
		}
		if endTime, ok := metadata["end_time"].(string); ok && endTime != "" {
			meetingInfo += "结束时间: " + endTime + "\n"
		}

		// 添加摘要
		if summary, ok := metadata["summary"].(string); ok && summary != "" {
			meetingInfo += "摘要: " + summary + "\n"
		}
	}

	// 合并会议信息和内容
	msg := meetingInfo + "\n会议内容:\n" + meetingContent

	// Set SSE headers
	c.Response.Header.Set("Content-Type", "text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")

	// Create SSE stream
	stream := sse.NewStream(c)

	// 使用会议信息和用户消息调用ChatMessage.Process
	res := models.ChatMessage{
		Data: msg,
	}.Process(message)

	data = []byte(res)

	event := &sse.Event{
		Data: data,
	}

	if err := stream.Publish(event); err != nil {
		return
	}
}
