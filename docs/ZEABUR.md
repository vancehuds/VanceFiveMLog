# Zeabur 部署教程

[English Version](ZEABUR.en.md)

本项目已支持 Zeabur 部署。Zeabur 会识别仓库根目录的 `Dockerfile` 构建 Go 服务，应用启动时会自动读取 Zeabur 注入的 `PORT` 端口。

## 前置准备

1. 将本项目推送到 GitHub、GitLab 或 Zeabur 支持的 Git 仓库。
2. 确认仓库根目录包含 `Dockerfile`。
3. 不需要上传 `.env`，生产环境变量在 Zeabur 控制台配置。

## 1. 创建 Zeabur 项目

1. 登录 Zeabur 控制台。
2. 创建一个新 Project。
3. 先添加一个 PostgreSQL 服务，等待数据库初始化完成。
4. 再添加一个 Git 服务，选择本项目仓库。
5. 构建方式选择 Dockerfile。如果 Zeabur 自动识别到根目录 `Dockerfile`，保持默认即可。

## 2. 配置环境变量

在 Git 服务的 Variables 中配置：

```env
APP_ENV=production
SESSION_SECRET=请替换为至少32位随机字符串
INITIAL_ADMIN_USERNAME=admin
INITIAL_ADMIN_PASSWORD=请替换为强密码
RETENTION_DAYS=180
APP_TIME_ZONE=Asia/Shanghai
TURNSTILE_SITE_KEY=
TURNSTILE_SECRET_KEY=
```

数据库连接变量配置二选一：

- **推荐**：把 PostgreSQL 服务提供的连接字符串绑定或复制为 `DATABASE_URL`。
- **兼容**：如果你的 Zeabur PostgreSQL 服务已注入 `POSTGRES_CONNECTION_STRING` 或 `POSTGRES_URI`，应用会自动读取。

**注意事项：**

- 不要在 Zeabur 中设置 `APP_ADDR`，让应用使用 Zeabur 注入的 `PORT`。
- `APP_ENV=production` 会启用更严格的会话密钥校验，并默认给后台会话 Cookie 添加 `Secure` 标记。
- `APP_TIME_ZONE` 使用 IANA 时区名，默认北京时间 `Asia/Shanghai`；也可以在后台"系统设置"修改。
- 配置 `TURNSTILE_SITE_KEY` 和 `TURNSTILE_SECRET_KEY` 后，登录页会启用 Cloudflare Turnstile 验证码；两个变量要么同时为空，要么同时填写。
- 如果连接字符串带 SSL 参数，保持 Zeabur 提供的原值即可。
- `INITIAL_ADMIN_*` 只会在数据库里没有管理员账号时创建首个管理员。已有管理员后，修改这两个变量不会重置密码。

## 3. 绑定域名并访问后台

1. 在 Git 服务的 Domains 中生成 Zeabur 默认域名，或绑定你自己的域名。
2. 打开域名，使用 `INITIAL_ADMIN_USERNAME` 和 `INITIAL_ADMIN_PASSWORD` 登录。
3. 进入"系统设置"，创建 FiveM 服务器并复制一次性 API Key。

## 4. 配置 FiveM Resource

把 `fivem-resource` 复制到 FiveM 服务器资源目录，并把目录命名为 `vancefivemlog`，例如：

```
resources/[logging]/vancefivemlog
```

**推荐**：在 FiveM 的 `server.cfg` 使用仅服务端可见的 convar 配置后台地址和 API Key，避免把密钥写进资源目录：

```cfg
set vfl_endpoint "https://你的-zeabur-域名/api/v1/events"
set vfl_heartbeat_endpoint "https://你的-zeabur-域名/api/v1/heartbeat"
set vfl_api_key "后台创建服务器时生成的一次性APIKey"
```

也可以直接修改 `fivem-resource/config.lua`：

```lua
Config.Endpoint = 'https://你的-zeabur-域名/api/v1/events'
Config.HeartbeatEndpoint = 'https://你的-zeabur-域名/api/v1/heartbeat'
Config.APIKey = '后台创建服务器时生成的一次性APIKey'
```

在 `server.cfg` 添加，放在上面的 `set` 配置之后：

```cfg
ensure vancefivemlog
```

重启 FiveM 服务器后，后台 Dashboard 应该能看到 heartbeat 状态和实时日志。

## 5. 排错

- **页面打不开**：检查 Git 服务是否成功构建并处于 Running 状态，确认没有手动设置错误的 `APP_ADDR`。
- **启动时报数据库连接变量缺失**：给 Git 服务配置 `DATABASE_URL`，或确认 PostgreSQL 服务变量已正确绑定到 Git 服务。
- **登录失败或没有管理员**：确认首次启动时设置了 `INITIAL_ADMIN_USERNAME` 和 `INITIAL_ADMIN_PASSWORD`。如果数据库已有管理员，请在后台"系统设置"中管理账号。
- **FiveM 无法写入日志**：确认 endpoint 使用公网 HTTPS 域名，API Key 没有多余空格，并查看 FiveM 控制台里的 HTTP 状态码。`status=0` 表示 FXServer 没拿到 HTTP 响应，通常是 DNS、TLS、防火墙或实际加载的 endpoint 配置问题。

## 6. 官方参考

- [Zeabur Deploy Go App](https://zeabur.com/docs/en-US/guides/go)
- [Zeabur Deploying with Dockerfile](https://zeabur.com/docs/en-US/deploy/methods/dockerfile)
