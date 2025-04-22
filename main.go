package main

import (
	"context"
	"time"

	"meetingagent/handlers"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

func main() {
	h := server.Default()
	h.Use(Logger())

	// 注册API路由
	h.POST("/meeting", handlers.CreateMeeting)
	h.GET("/meeting", handlers.ListMeetings)
	h.GET("/summary", handlers.GetMeetingSummary)
	h.GET("/mermaid", handlers.GetMeetingMermaid)
	h.GET("/score", handlers.GetMeetingScore)
	h.GET("/chat", handlers.HandleChat)
	h.GET("/roleplay", handlers.HandleRolePlayChat)
	h.GET("/push-report", handlers.PushMeetingReport)

	// 注册多角色扮演会议路由
	h.POST("/multi-roleplay", handlers.HandleMultiRoleplayMeeting)
	h.POST("/multi-roleplay/stream", handlers.HandleStreamMultiRoleplayMeeting)

	// 注册待办事项路由
	h.POST("/todo", handlers.CreateTodo)
	h.GET("/todo", handlers.GetTodoList)
	h.PUT("/todo/:id", handlers.UpdateTodo)
	h.DELETE("/todo/:id", handlers.DeleteTodo)

	// 提供静态文件服务
	h.StaticFS("/", &app.FS{
		Root:               "./static",
		PathRewrite:        app.NewPathSlashesStripper(1),
		IndexNames:         []string{"index.html"},
		GenerateIndexPages: true,
	})

	// 启动服务器
	h.Spin()
}

// Logger 请求日志中间件
func Logger() app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		start := time.Now()
		path := string(ctx.Request.URI().Path())
		query := string(ctx.Request.URI().QueryString())
		if query != "" {
			path = path + "?" + query
		}

		// 处理请求
		ctx.Next(c)

		// 计算耗时
		latency := time.Since(start)

		// 获取响应状态码
		statusCode := ctx.Response.StatusCode()

		// 记录请求详情
		hlog.CtxInfof(c, "[HTTP] %s %s - %d - %v",
			ctx.Request.Method(),
			path,
			statusCode,
			latency,
		)
	}
}
