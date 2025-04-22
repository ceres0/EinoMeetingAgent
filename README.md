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

## API_KEY 说明

- 请在 config/config.json.template 中配置 API_KEY
- 配置完成后，将 config/config.json.template 重命名为 config/config.json
