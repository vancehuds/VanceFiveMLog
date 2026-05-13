# 贡献指南

感谢您对 VanceFiveMLog 的贡献与支持。

[English Version](CONTRIBUTING.en.md)

## 开发环境

1. 复制 `.env.example` 到 `.env` 并调整本地配置。
2. 运行 `docker compose up --build` 启动本地 PostgreSQL 后端应用。
3. 提交 Pull Request 前运行 `go test ./...`。

## Pull Request 规范

- 保持更改专注且简洁，描述用户可见的行为变化。
- 包含您运行的验证命令。
- 更改配置时更新文档和 `.env.example`。
- UI 更改请附上截图。

## 代码风格

- 遵循 Go 语言习惯和格式规范 (`go fmt`, `go vet`)
- 保持函数小巧专注
- 为非显而易见逻辑添加注释
- 编写有意义的提交信息

## 测试

- 提交前运行 `go test ./...`
- 为新功能添加测试
- 确保所有现有测试通过

## 文档

- 更改功能时更新相关文档
- 为新 API 端点添加示例
- 保持 README 和集成指南最新

## 许可证

您同意您的贡献将遵循 `AGPL-3.0-or-later` 许可证授权。
