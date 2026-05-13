# 安全政策

[English Version](SECURITY.en.md)

## 支持版本

除非明确宣布发布分支，否则安全修复仅发布到最新 `main` 分支。

| 版本     | 支持状态           |
| -------- | ------------------ |
| main     | :white_check_mark: |
| 更早版本 | :x:                |

## 报告安全漏洞

请勿为疑似安全漏洞开启公开 Issues。

请通过 GitHub 安全咨询报告：

https://github.com/vancehuds/VanceFiveMLog/security/advisories/new

请包含以下信息：

- 受影响版本或提交哈希
- 部署模式和环境
- 复现步骤
- 影响说明及任何有助于解释问题的日志或截图

如果 GitHub 安全咨询不可用，请在公开发布详细信息之前私下联系仓库所有者。

## 安全最佳实践

### 生产环境部署

- 设置 `APP_ENV=production` 以启用安全会话 Cookie
- 使用至少 32 个字符的唯一 `SESSION_SECRET`
- 生产环境启用 TLS/HTTPS
- 正确配置防火墙规则

### 管理员安全

- 为管理员账户使用强密码
- 定期轮换 `SESSION_SECRET`
- 启用 Cloudflare Turnstile 增加登录保护
- 监控日志审核工作流中的可疑活动

### API 密钥

- 服务器 API 密钥以 SHA-256 哈希形式存储
- 保持 API 密钥机密，泄露时及时轮换
- 使用环境变量或安全配置存储 API 密钥

## 依赖项

本项目采用 Go 构建，使用以下安全相关依赖：

- `golang.org/x/crypto` - 用于安全密码哈希和会话管理

请通过 GitHub 安全咨询报告依赖项中的任何漏洞。
