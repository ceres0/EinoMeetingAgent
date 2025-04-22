# Meeting Agent

## 项目简介

Meeting Agent 是一个用于会议管理的 API 服务，基于 Golang 和 Hertz 框架开发。该项目提供了创建会议、获取会议列表、生成会议摘要、多角色扮演对话以及待办事项管理等功能，旨在提高会议效率和管理体验。

## 功能简介

- **会议管理**：创建会议、查看会议列表
- **会议摘要**：自动生成会议内容摘要
- **图表生成**：支持生成会议内容的 Mermaid 图表
- **会议评分**：对会议质量进行评分
- **实时聊天**：支持基于 SSE (Server-Sent Events) 的实时聊天功能
- **角色扮演**：支持单角色和多角色扮演会议模式
- **待办事项**：创建、获取、更新和删除待办事项
- **报告推送**：支持会议报告推送功能

## 配置文件说明

- 请在 config/config.json.template 中配置 API_KEY、FEISHU_WEBHOOK_URL
- 配置完成后，将 config/config.json.template 重命名为 config/config.json

## 环境要求与项目运行

### 环境要求

- Go 版本：1.24.2
- 依赖管理：项目使用 Go Modules 进行依赖管理

### 项目运行

1. 克隆仓库后，进入项目根目录
2. 执行以下命令下载所需依赖:

   ```bash
   go mod tidy
   ```

3. 启动服务:

   ```bash
   go run main.go
   ```

## 接口测试

项目提供了 Postman 接口测试集合，可按照以下步骤进行测试：

1. 导入项目根目录下的 `CloseAI_postman_collection.json` 文件到 Postman
2. 确保服务已经启动
3. 在 Postman 中执行相关接口测试

## 接口文档

完整的接口说明详见项目根目录下的 `interface_README.md` 文件，其中包含了所有API的详细说明、请求参数、响应格式和curl示例。

## 项目目录结构

项目主要目录结构及功能说明：

- `config/`: 配置文件目录，包含API密钥和飞书Webhook等配置信息
- `handlers/`: API请求处理器，负责接收和处理HTTP请求
  - `meeting.go`: 会议相关接口处理
  - `todo.go`: 待办事项相关接口处理
- `models/`: 数据模型和业务逻辑
  - `meeting.go`: 会议相关数据模型和功能实现
  - `multi_roleplay_meeting.go`: 多角色扮演会议的实现
  - `config.go`: 配置相关的数据结构定义
- `storage/`: 数据存储相关
  - `meetings/`: 会议数据存储目录，以JSON文件形式保存
  - `todo.db`: SQLite数据库文件，用于存储待办事项
- `sql/`: 数据库操作相关代码
  - `sqlite.go`: SQLite数据库操作的实现
- `test_data/`: 测试数据，包含各种测试用例
- `static/`: 静态资源文件
- `example/`: 示例代码和使用案例

## 数据存储说明

### 测试数据

- 项目在 `test_data/` 目录下提供了多个JSON格式的测试数据文件，可用于功能测试和开发调试
- 测试数据包含不同类型的会议内容，涵盖了多种会议场景

### 数据存储

- 会议数据：以JSON格式存储在 `storage/meetings/` 目录下，文件名格式为 `meeting_yyyyMMddHHmmss.json`
- 待办事项：使用SQLite数据库存储在 `storage/todo.db` 文件中
- 数据库结构和操作逻辑可参考 `sql/sqlite.go` 文件
