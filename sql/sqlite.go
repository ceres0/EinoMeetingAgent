package sql

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/glebarez/go-sqlite" // 引入纯Go实现的sqlite驱动
)

// Todo 表示一个待办事项
type Todo struct {
	ID          int64     `json:"id"`          // 待办事项ID
	Title       string    `json:"title"`       // 待办事项标题
	Description string    `json:"description"` // 待办事项描述
	Status      string    `json:"status"`      // 待办事项状态（未开始、进行中、已完成）
	Priority    int       `json:"priority"`    // 优先级（1-高，2-中，3-低）
	DueDate     time.Time `json:"due_date"`    // 截止日期
	CreatedAt   time.Time `json:"created_at"`  // 创建时间
	UpdatedAt   time.Time `json:"updated_at"`  // 更新时间
	MeetingID   string    `json:"meeting_id"`  // 关联的会议ID
	AssignedTo  string    `json:"assigned_to"` // 分配给谁
}

func openDatabase(dbName string) (*sql.DB, error) {
	// 检查数据库文件是否存在，如果不存在则创建
	if _, err := os.Stat(dbName); os.IsNotExist(err) {
		fmt.Println("数据库文件不存在，将创建:", dbName)

		// 确保目录存在
		dir := filepath.Dir(dbName)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("创建数据库目录失败: %w", err)
			}
		}
	}

	db, err := sql.Open("sqlite", dbName) // 使用sqlite驱动
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	err = db.Ping() // 尝试连接数据库
	if err != nil {
		db.Close() // 连接失败时关闭连接
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	fmt.Println("成功连接到数据库:", dbName)
	return db, nil
}

// InitTodoTable 初始化Todo表
func InitTodoTable(dbName string) error {
	db, err := openDatabase(dbName)
	if err != nil {
		return err
	}
	defer db.Close()

	// 启用外键约束
	_, err = db.Exec("PRAGMA foreign_keys = ON;")
	if err != nil {
		return fmt.Errorf("启用外键约束失败: %w", err)
	}

	// 创建Todo表
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS todos (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		description TEXT,
		status TEXT NOT NULL DEFAULT '未开始',
		priority INTEGER DEFAULT 3,
		due_date TIMESTAMP,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		meeting_id TEXT,
		assigned_to TEXT
	);
	`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("创建Todo表失败: %w", err)
	}

	fmt.Println("成功初始化Todo表")
	return nil
}

// AddTodo 添加一个新的待办事项
func AddTodo(dbName string, todo *Todo) (int64, error) {
	db, err := openDatabase(dbName)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	// 设置创建时间和更新时间为当前时间
	now := time.Now()
	todo.CreatedAt = now
	todo.UpdatedAt = now

	// 插入数据
	insertSQL := `
	INSERT INTO todos (
		title, description, status, priority, due_date, 
		created_at, updated_at, meeting_id, assigned_to
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
	`

	result, err := db.Exec(insertSQL,
		todo.Title, todo.Description, todo.Status, todo.Priority, todo.DueDate,
		todo.CreatedAt, todo.UpdatedAt, todo.MeetingID, todo.AssignedTo)
	if err != nil {
		return 0, fmt.Errorf("添加待办事项失败: %w", err)
	}

	// 获取新插入行的ID
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取新增待办事项ID失败: %w", err)
	}

	todo.ID = id
	return id, nil
}

// GetTodoByID 根据ID获取待办事项
func GetTodoByID(dbName string, id int64) (*Todo, error) {
	db, err := openDatabase(dbName)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// 使用ListTodos来避免直接查询的问题
	todos, err := ListTodos(dbName, "", "", 0)
	if err != nil {
		return nil, fmt.Errorf("查询待办事项列表失败: %w", err)
	}

	// 在内存中查找匹配的ID
	for _, todo := range todos {
		if todo.ID == id {
			return todo, nil
		}
	}

	return nil, fmt.Errorf("找不到ID为%d的待办事项", id)
}

// UpdateTodo 更新待办事项
func UpdateTodo(dbName string, todo *Todo) error {
	db, err := openDatabase(dbName)
	if err != nil {
		return err
	}
	defer db.Close()

	// 设置更新时间为当前时间
	todo.UpdatedAt = time.Now()

	// 更新数据，使用索引占位符
	updateSQL := `
	UPDATE todos
	SET title = ?1, description = ?2, status = ?3, priority = ?4, due_date = ?5,
	    updated_at = ?6, meeting_id = ?7, assigned_to = ?8
	WHERE id = ?9;
	`

	result, err := db.Exec(updateSQL,
		todo.Title, todo.Description, todo.Status, todo.Priority, todo.DueDate,
		todo.UpdatedAt, todo.MeetingID, todo.AssignedTo, todo.ID)
	if err != nil {
		return fmt.Errorf("更新待办事项失败: %w", err)
	}

	// 检查是否更新了任何行
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取更新行数失败: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("找不到ID为%d的待办事项", todo.ID)
	}

	return nil
}

// DeleteTodo 删除待办事项
func DeleteTodo(dbName string, id int64) error {
	db, err := openDatabase(dbName)
	if err != nil {
		return err
	}
	defer db.Close()

	// 删除数据，使用索引占位符
	deleteSQL := `DELETE FROM todos WHERE id = ?1;`

	result, err := db.Exec(deleteSQL, id)
	if err != nil {
		return fmt.Errorf("删除待办事项失败: %w", err)
	}

	// 检查是否删除了任何行
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取删除行数失败: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("找不到ID为%d的待办事项", id)
	}

	return nil
}

// ListTodos 列出所有待办事项，可以根据条件筛选
func ListTodos(dbName string, meetingID string, status string, priority int) ([]*Todo, error) {
	db, err := openDatabase(dbName)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// 构建查询条件，使用索引占位符
	querySQL := `
	SELECT id, title, description, status, priority, due_date, 
	       created_at, updated_at, meeting_id, assigned_to
	FROM todos
	WHERE 1=1
	`
	var args []interface{}
	paramIndex := 1

	if meetingID != "" {
		querySQL += fmt.Sprintf(" AND meeting_id = ?%d", paramIndex)
		args = append(args, meetingID)
		paramIndex++
	}

	if status != "" {
		querySQL += fmt.Sprintf(" AND status = ?%d", paramIndex)
		args = append(args, status)
		paramIndex++
	}

	if priority > 0 {
		querySQL += fmt.Sprintf(" AND priority = ?%d", paramIndex)
		args = append(args, priority)
		paramIndex++
	}

	querySQL += " ORDER BY priority ASC, due_date ASC;"

	// 执行查询
	rows, err := db.Query(querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("查询待办事项列表失败: %w", err)
	}
	defer rows.Close()

	// 遍历结果集
	var todos []*Todo
	for rows.Next() {
		var todo Todo
		var dueDate sql.NullTime // 处理NULL值

		err := rows.Scan(
			&todo.ID, &todo.Title, &todo.Description, &todo.Status, &todo.Priority,
			&dueDate, &todo.CreatedAt, &todo.UpdatedAt, &todo.MeetingID, &todo.AssignedTo,
		)
		if err != nil {
			return nil, fmt.Errorf("读取待办事项数据失败: %w", err)
		}

		// 如果截止日期不为空，则设置到todo结构体中
		if dueDate.Valid {
			todo.DueDate = dueDate.Time
		}

		todos = append(todos, &todo)
	}

	// 检查遍历过程中是否有错误
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历待办事项数据失败: %w", err)
	}

	return todos, nil
}

// GetTodosByMeetingID 根据会议ID获取相关待办事项
func GetTodosByMeetingID(dbName string, meetingID string) ([]*Todo, error) {
	return ListTodos(dbName, meetingID, "", 0)
}

// GetTodosByStatus 根据状态获取待办事项
func GetTodosByStatus(dbName string, status string) ([]*Todo, error) {
	return ListTodos(dbName, "", status, 0)
}

// GetTodosByPriority 根据优先级获取待办事项
func GetTodosByPriority(dbName string, priority int) ([]*Todo, error) {
	return ListTodos(dbName, "", "", priority)
}

// BatchAddTodos 批量添加待办事项
func BatchAddTodos(dbName string, todos []*Todo) error {
	db, err := openDatabase(dbName)
	if err != nil {
		return err
	}
	defer db.Close()

	// 开始事务
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}

	// 准备插入语句，使用索引占位符
	insertSQL := `
	INSERT INTO todos (
		title, description, status, priority, due_date, 
		created_at, updated_at, meeting_id, assigned_to
	) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9);
	`

	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("准备插入语句失败: %w", err)
	}
	defer stmt.Close()

	// 设置当前时间
	now := time.Now()

	// 批量执行插入
	for _, todo := range todos {
		// 设置创建时间和更新时间
		todo.CreatedAt = now
		todo.UpdatedAt = now

		_, err := stmt.Exec(
			todo.Title, todo.Description, todo.Status, todo.Priority, todo.DueDate,
			todo.CreatedAt, todo.UpdatedAt, todo.MeetingID, todo.AssignedTo,
		)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("批量插入待办事项失败: %w", err)
		}
	}

	// 提交事务
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	return nil
}
