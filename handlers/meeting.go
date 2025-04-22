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
	sqldb "meetingagent/sql"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/hertz-contrib/sse"
)

// CreateMeeting 处理创建会议请求
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

	// 将会议中的待办事项添加到数据库
	if todoList, ok := meetingInfo["todo_list"].([]interface{}); ok && len(todoList) > 0 {
		// 提取会议标题作为任务描述前缀
		meetingTitle := ""
		if title, ok := meetingInfo["title"].(string); ok {
			meetingTitle = title
		}

		// 将todo_list中的每一项添加到数据库
		var todos []*sqldb.Todo
		for _, item := range todoList {
			if todoStr, ok := item.(string); ok && todoStr != "" {
				todo := &sqldb.Todo{
					Title:       todoStr,
					Description: fmt.Sprintf("来自会议: %s", meetingTitle),
					Status:      "未开始",
					Priority:    2, // 默认中等优先级
					MeetingID:   meetingID,
				}
				todos = append(todos, todo)
			}
		}

		// 批量添加待办事项
		if len(todos) > 0 {
			todoDbName := "./storage/todo.db"
			if err := sqldb.BatchAddTodos(todoDbName, todos); err != nil {
				fmt.Printf("添加会议待办事项失败: %v\n", err)
				// 这里我们只记录错误，不中断会议创建流程
			} else {
				fmt.Printf("成功添加 %d 个会议待办事项到数据库\n", len(todos))
			}
		}
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

// ListMeetings 处理获取会议列表请求
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

// GetMeetingSummary 处理获取会议摘要请求
func GetMeetingSummary(ctx context.Context, c *app.RequestContext) {
	meetingID := c.Query("meeting_id")
	if meetingID == "" {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "meeting_id is required"})
		return
	}
	fmt.Printf("meetingID: %s\n", meetingID)

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

	// 从meetingData中提取摘要信息
	var summary string

	// 尝试从新格式中获取元数据
	if metadata, ok := meetingData["metadata"].(map[string]interface{}); ok {
		// 提取摘要
		if sum, ok := metadata["summary"].(string); ok {
			summary = sum
		} else {
			summary = "无摘要信息"
		}
	} else {
		// 兼容旧格式，或者使用整个数据
		summary = "无法从会议数据中提取摘要信息"
	}

	// 构建响应
	response := map[string]interface{}{
		"summary": summary,
	}

	c.JSON(consts.StatusOK, response)
}

// HandleChat 处理SSE聊天会话
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

		// 添加任务
		// 添加参会人员
		if todo_list, ok := metadata["todo_list"].([]interface{}); ok && len(todo_list) > 0 {
			meetingInfo += "会议任务: "
			for i, p := range todo_list {
				if i > 0 {
					meetingInfo += ", "
				}
				if pStr, ok := p.(string); ok {
					meetingInfo += pStr
				}
			}
			meetingInfo += "\n"
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

	// 使用会议信息和用户消息调用ChatMessage.Process进行流式处理
	chatMsg := models.ChatMessage{
		Data: msg,
	}
	if err := chatMsg.Process(message, stream, meetingID, sessionID); err != nil {
		c.AbortWithStatus(consts.StatusInternalServerError)
		return
	}
}

// GetMeetingMermaid 处理获取会议流程图请求
func GetMeetingMermaid(ctx context.Context, c *app.RequestContext) {
	meetingID := c.Query("meeting_id")
	if meetingID == "" {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "meeting_id is required"})
		return
	}
	fmt.Printf("处理会议流程图请求，meetingID: %s\n", meetingID)

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

	// 调用ExtractMermaid生成流程图
	mermaidCode, err := models.ExtractMermaid(ctx, meetingContent)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, utils.H{"error": "生成流程图失败: " + err.Error()})
		return
	}

	// 构建响应
	response := map[string]interface{}{
		"mermaid_code": mermaidCode,
	}

	c.JSON(consts.StatusOK, response)
}

// HandleRolePlayChat 处理角色扮演聊天会话
func HandleRolePlayChat(ctx context.Context, c *app.RequestContext) {
	meetingID := c.Query("meeting_id")
	sessionID := c.Query("session_id")
	message := c.Query("message")
	participantName := c.Query("participant")

	if meetingID == "" || sessionID == "" {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "meeting_id and session_id are required"})
		return
	}

	if message == "" {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "message is required"})
		return
	}

	if participantName == "" {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "participant is required"})
		return
	}

	fmt.Printf("角色扮演聊天: meetingID: %s, sessionID: %s, participant: %s, message: %s\n",
		meetingID, sessionID, participantName, message)

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

	// 使用会议信息和用户消息调用RolePlayMessage.ProcessRolePlay进行流式处理
	rolePlayMsg := models.RolePlayMessage{
		Data:            msg,
		ParticipantName: participantName,
	}
	if err := rolePlayMsg.ProcessRolePlay(message, stream); err != nil {
		c.AbortWithStatus(consts.StatusInternalServerError)
		return
	}
}

// GetMeetingScore 处理获取会议评分请求
func GetMeetingScore(ctx context.Context, c *app.RequestContext) {
	meetingID := c.Query("meeting_id")
	if meetingID == "" {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "meeting_id is required"})
		return
	}
	fmt.Printf("处理会议评分请求，meetingID: %s\n", meetingID)

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

	// 获取会议元数据并添加到内容中，提供更多上下文
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

		// 添加摘要
		if summary, ok := metadata["summary"].(string); ok && summary != "" {
			meetingInfo += "摘要: " + summary + "\n"
		}
	}

	// 合并会议信息和内容
	fullContent := meetingInfo + "\n会议内容:\n" + meetingContent

	// 调用EvaluateMeeting评估会议
	meetingScore, err := models.EvaluateMeeting(ctx, fullContent)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, utils.H{"error": "评估会议失败: " + err.Error()})
		return
	}

	// 返回评分结果
	c.JSON(consts.StatusOK, meetingScore)
}

// PushMeetingReport 处理推送会议报告到飞书的请求
func PushMeetingReport(ctx context.Context, c *app.RequestContext) {
	// 获取会议ID
	meetingID := c.Query("meeting_id")
	if meetingID == "" {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "meeting_id是必需的"})
		return
	}

	fmt.Printf("推送会议报告到飞书, meetingID: %s\n", meetingID)

	// 推送会议报告到飞书
	if err := models.PushMeetingReportToFeiShu(meetingID); err != nil {
		c.JSON(consts.StatusInternalServerError, utils.H{"error": fmt.Sprintf("推送会议报告失败: %v", err)})
		return
	}

	// 返回成功响应
	c.JSON(consts.StatusOK, utils.H{
		"message": "会议报告已成功推送到飞书",
	})
}

// HandleMultiRoleplayMeeting 处理多角色扮演会议请求
func HandleMultiRoleplayMeeting(ctx context.Context, c *app.RequestContext) {
	// 获取请求参数
	var reqBody models.MultiRoleplayRequest
	if err := c.BindJSON(&reqBody); err != nil {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "无效的请求体: " + err.Error()})
		return
	}

	// 参数验证
	if reqBody.MeetingID == "" {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "meeting_id 是必需的"})
		return
	}

	if reqBody.Host == "" {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "host 是必需的"})
		return
	}

	if len(reqBody.Specialists) == 0 {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "至少需要一名专家参与者"})
		return
	}

	if reqBody.Rounds <= 0 {
		reqBody.Rounds = 3 // 默认进行3轮讨论
	}

	// 执行多角色扮演会议
	response, err := models.PerformMultiRoleplayMeeting(&reqBody)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, utils.H{"error": "执行多角色扮演会议失败: " + err.Error()})
		return
	}

	// 返回结果
	c.JSON(consts.StatusOK, response)
}

// HandleStreamMultiRoleplayMeeting 处理流式多角色扮演会议请求
func HandleStreamMultiRoleplayMeeting(ctx context.Context, c *app.RequestContext) {
	// 获取请求参数
	var reqBody models.MultiRoleplayRequest
	if err := c.BindJSON(&reqBody); err != nil {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "无效的请求体: " + err.Error()})
		return
	}

	// 参数验证
	if reqBody.MeetingID == "" {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "meeting_id 是必需的"})
		return
	}

	if reqBody.Host == "" {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "host 是必需的"})
		return
	}

	if len(reqBody.Specialists) == 0 {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "至少需要一名专家参与者"})
		return
	}

	if reqBody.Rounds <= 0 {
		reqBody.Rounds = 3 // 默认进行3轮讨论
	}

	// 设置SSE响应头
	c.Response.Header.Set("Content-Type", "text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")

	// 创建SSE流
	stream := sse.NewStream(c)

	// 流式执行多角色扮演会议
	if err := models.StreamMultiRoleplayMeeting(ctx, &reqBody, stream); err != nil {
		c.AbortWithStatus(consts.StatusInternalServerError)
		return
	}
}
