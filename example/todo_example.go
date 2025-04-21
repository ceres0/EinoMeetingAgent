package main

import (
	"fmt"
	"log"
	"os"
	"time"

	sqlitedb "meetingagent/sql" // 引入我们的sql包
)

const dbName = "./test_data/todo.db"

func main() {
	// 确保测试数据目录存在
	if err := ensureDir("./test_data"); err != nil {
		log.Fatalf("创建测试数据目录失败: %v", err)
	}

	// 初始化Todo表
	if err := sqlitedb.InitTodoTable(dbName); err != nil {
		log.Fatalf("初始化Todo表失败: %v", err)
	}

	// 添加Todo示例
	addTodoExample()

	// 查询Todo示例
	queryTodoExample()

	// 更新Todo示例
	updateTodoExample()

	// 删除Todo示例
	deleteTodoExample()

	// 批量添加Todo示例
	batchAddTodoExample()

	fmt.Println("所有示例执行完毕！")
}

// 添加Todo示例
func addTodoExample() {
	fmt.Println("\n===== 添加Todo示例 =====")

	// 创建一个新的Todo
	todo := &sqlitedb.Todo{
		Title:       "准备会议材料",
		Description: "为下周的产品讨论会准备演示文稿和演示材料",
		Status:      "未开始",
		Priority:    1,                              // 高优先级
		DueDate:     time.Now().Add(72 * time.Hour), // 3天后截止
		MeetingID:   "meeting123",
		AssignedTo:  "张三",
	}

	// 添加到数据库
	id, err := sqlitedb.AddTodo(dbName, todo)
	if err != nil {
		log.Printf("添加Todo失败: %v", err)
		return
	}

	fmt.Printf("成功添加Todo，ID: %d\n", id)
}

// 查询Todo示例
func queryTodoExample() {
	fmt.Println("\n===== 查询Todo示例 =====")

	// 列出所有Todo
	todos, err := sqlitedb.ListTodos(dbName, "", "", 0)
	if err != nil {
		log.Printf("查询Todo列表失败: %v", err)
		return
	}

	fmt.Printf("共查询到 %d 个Todo项\n", len(todos))
	for i, todo := range todos {
		fmt.Printf("%d. %s (优先级: %d, 状态: %s, 负责人: %s)\n",
			i+1, todo.Title, todo.Priority, todo.Status, todo.AssignedTo)
	}

	// 根据ID查询特定Todo
	if len(todos) > 0 {
		id := todos[0].ID
		todo, err := sqlitedb.GetTodoByID(dbName, id)
		if err != nil {
			log.Printf("根据ID查询Todo失败: %v", err)
			return
		}

		fmt.Printf("\n找到ID为 %d 的Todo:\n", id)
		fmt.Printf("标题: %s\n", todo.Title)
		fmt.Printf("描述: %s\n", todo.Description)
		fmt.Printf("状态: %s\n", todo.Status)
		fmt.Printf("优先级: %d\n", todo.Priority)
		fmt.Printf("截止日期: %s\n", todo.DueDate.Format("2006-01-02 15:04:05"))
		fmt.Printf("负责人: %s\n", todo.AssignedTo)
	}

	// 根据会议ID查询Todo
	meetingTodos, err := sqlitedb.GetTodosByMeetingID(dbName, "meeting123")
	if err != nil {
		log.Printf("根据会议ID查询Todo失败: %v", err)
		return
	}

	fmt.Printf("\n会议ID为 'meeting123' 的Todo数量: %d\n", len(meetingTodos))
}

// 更新Todo示例
func updateTodoExample() {
	fmt.Println("\n===== 更新Todo示例 =====")

	// 先获取所有Todo
	todos, err := sqlitedb.ListTodos(dbName, "", "", 0)
	if err != nil || len(todos) == 0 {
		log.Printf("没有找到可更新的Todo: %v", err)
		return
	}

	// 更新第一个Todo
	todo := todos[0]
	fmt.Printf("更新前: %s (状态: %s, 优先级: %d)\n", todo.Title, todo.Status, todo.Priority)

	// 修改状态和优先级
	todo.Status = "进行中"
	todo.Priority = 2
	todo.Description = todo.Description + " [已更新]"

	// 保存更新
	if err := sqlitedb.UpdateTodo(dbName, todo); err != nil {
		log.Printf("更新Todo失败: %v", err)
		return
	}

	// 重新获取检查更新是否成功
	updatedTodo, err := sqlitedb.GetTodoByID(dbName, todo.ID)
	if err != nil {
		log.Printf("获取更新后的Todo失败: %v", err)
		return
	}

	fmt.Printf("更新后: %s (状态: %s, 优先级: %d)\n", updatedTodo.Title, updatedTodo.Status, updatedTodo.Priority)
}

// 删除Todo示例
func deleteTodoExample() {
	fmt.Println("\n===== 删除Todo示例 =====")

	// 添加一个临时的Todo以便删除
	tempTodo := &sqlitedb.Todo{
		Title:       "临时任务",
		Description: "这是一个将被删除的临时任务",
		Status:      "未开始",
		MeetingID:   "meeting_temp",
	}

	// 添加到数据库
	id, err := sqlitedb.AddTodo(dbName, tempTodo)
	if err != nil {
		log.Printf("添加临时Todo失败: %v", err)
		return
	}

	fmt.Printf("添加了临时Todo，ID: %d\n", id)

	// 获取添加前的所有Todo数量
	beforeTodos, _ := sqlitedb.ListTodos(dbName, "", "", 0)
	fmt.Printf("删除前共有 %d 个Todo项\n", len(beforeTodos))

	// 删除这个临时Todo
	if err := sqlitedb.DeleteTodo(dbName, id); err != nil {
		log.Printf("删除Todo失败: %v", err)
		return
	}

	// 获取删除后的所有Todo数量
	afterTodos, _ := sqlitedb.ListTodos(dbName, "", "", 0)
	fmt.Printf("删除后共有 %d 个Todo项\n", len(afterTodos))

	// 尝试获取已删除的Todo
	_, err = sqlitedb.GetTodoByID(dbName, id)
	if err != nil {
		fmt.Printf("预期的错误: %v\n", err)
	}
}

// 批量添加Todo示例
func batchAddTodoExample() {
	fmt.Println("\n===== 批量添加Todo示例 =====")

	// 创建多个Todo
	todos := []*sqlitedb.Todo{
		{
			Title:       "编写文档",
			Description: "编写API文档",
			Status:      "未开始",
			Priority:    2,
			MeetingID:   "meeting456",
			AssignedTo:  "李四",
		},
		{
			Title:       "代码审核",
			Description: "审核前端代码",
			Status:      "未开始",
			Priority:    1,
			MeetingID:   "meeting456",
			AssignedTo:  "王五",
		},
		{
			Title:       "单元测试",
			Description: "编写单元测试用例",
			Status:      "未开始",
			Priority:    3,
			MeetingID:   "meeting456",
			AssignedTo:  "赵六",
		},
	}

	// 获取添加前的所有Todo数量
	beforeTodos, _ := sqlitedb.ListTodos(dbName, "", "", 0)
	fmt.Printf("批量添加前共有 %d 个Todo项\n", len(beforeTodos))

	// 批量添加
	if err := sqlitedb.BatchAddTodos(dbName, todos); err != nil {
		log.Printf("批量添加Todo失败: %v", err)
		return
	}

	// 获取添加后的所有Todo数量
	afterTodos, _ := sqlitedb.ListTodos(dbName, "", "", 0)
	fmt.Printf("批量添加后共有 %d 个Todo项\n", len(afterTodos))

	// 查询特定会议的Todo
	meetingTodos, err := sqlitedb.GetTodosByMeetingID(dbName, "meeting456")
	if err != nil {
		log.Printf("查询会议Todo失败: %v", err)
		return
	}

	fmt.Printf("会议ID为 'meeting456' 的Todo数量: %d\n", len(meetingTodos))
	for i, todo := range meetingTodos {
		fmt.Printf("%d. %s (负责人: %s, 优先级: %d)\n",
			i+1, todo.Title, todo.AssignedTo, todo.Priority)
	}
}

// 确保目录存在
func ensureDir(dirName string) error {
	if _, err := os.Stat(dirName); os.IsNotExist(err) {
		return os.MkdirAll(dirName, 0755)
	}
	return nil
}
