# VanceFiveMLog

[English Version](README.en.md) | 中文说明

[![License: AGPL-3.0-or-later](https://img.shields.io/badge/License-AGPL--3.0--or--later-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.25-blue)](go.mod)

Go + PostgreSQL FiveM/Qbox 日志浏览器，采用密集型 HTMX 风格管理后台，支持插件 API 密钥、SSE 实时日志和 Qbox 转发资源。

## 功能特性

- **管理员管理**：签名 HTTP-Only 会话 Cookie 登录，CSRF 保护
- **多服务器支持**：每服务器 API 密钥，以 SHA-256 哈希存储
- **批量日志摄入**：高效接收 FiveM/Qbox 资源日志
- **服务器健康监控**：心跳追踪，显示在线、过期、离线、禁用状态
- **实时日志**：仪表盘 SSE 实时日志流
- **仪表盘分析**：24 小时事件趋势和热门事件类型排行
- **高级搜索**：按服务器、事件类型、严重级别、资源、玩家、标识符、消息、元数据JSON、审核状态、归档状态和时间范围筛选
- **审核工作流**：日志审核，支持正常、可疑、违规状态，管理员备注
- **AI JSON 解析**：导入日志样本并生成展示方法（仅所有者可用）
- **CSV 导出**：包含审核和归档字段的日志导出
- **时区支持**：可配置的显示/查询时区（默认：亚洲/上海）
- **玩家时间线**：玩家行为时间线可视化
- **账号审计**：账号关联审计追踪
- **交互式地图**：Los Santos 坐标地图，支持平移、缩放、标记聚焦和轨迹线条
- **可配置保留期**：默认 180 天日志保留
- **事件桥接**：内置支持 Qbox/QBCore、背包、载具和 txAdmin 事件

## 快速开始

```bash
cp .env.example .env
docker compose up --build
```

打开 `http://localhost:8080`，使用 `.env` 中的 `INITIAL_ADMIN_*` 值登录。

## 部署

- Zeabur：详见 [docs/ZEABUR.md](docs/ZEABUR.md)
- 插件集成：详见 [docs/INTEGRATION.md](docs/INTEGRATION.md)

## FiveM Resource 配置

1. 打开**系统设置**。
2. 创建 FiveM 服务器并复制一次性 API Key。
3. 将 `fivem-resource` 复制到 FiveM 资源目录并命名为 `vancefivemlog`，例如：`resources/[logging]/vancefivemlog`。
4. 编辑 `vancefivemlog/config.lua`，填写后端事件 URL、心跳 URL 和 API Key。
5. 在 `server.cfg` 中添加 `ensure vancefivemlog`。

## API 参考

### 事件摄入

```http
POST /api/v1/events
Authorization: Bearer <server_api_key>
Content-Type: application/json
```

### 心跳

```http
POST /api/v1/heartbeat
Authorization: Bearer <server_api_key>
Content-Type: application/json
```

### 事件载荷

```json
{
  "events": [
    {
      "event_type": "inventory_remove",
      "severity": "warning",
      "source": 12,
      "player_name": "Vance",
      "license": "license:...",
      "citizenid": "ABC12345",
      "resource": "inventory",
      "message": "removed marked bills",
      "coords": { "x": 123.4, "y": 456.7, "z": 78.9 },
      "metadata": { "item": "markedbills", "amount": 5 }
    }
  ]
}
```

插件导向事件（无 `events` 包装）：

```json
{
  "event": "door forced",
  "level": "warning",
  "player_source": 12,
  "plugin_resource": "doorlocks",
  "message": "front bank door forced open",
  "data": { "door_id": "bank-front" }
}
```

### FiveM Export

对于服务端集成，推荐使用 FiveM export 以自动补全玩家标识符：

```lua
exports.vancefivemlog:Log('door forced', 'front bank door forced open', {
  severity = 'warning',
  source = source,
  metadata = { door_id = 'bank-front' }
})
```

## 本地开发

本地运行 PostgreSQL 或使用 Docker，然后：

```bash
go test ./...
go run ./cmd/server
```

在设置页面创建服务器 API Key 后，填充示例事件：

```bash
VFL_API_KEY=vfl_xxx go run ./cmd/seed
```

## 管理员安全

所有认证管理员的 POST 表单都包含 CSRF 保护。首个管理员仅在数据库无管理员行时从 `INITIAL_ADMIN_*` 创建。额外管理员可由 `owner` 在**系统设置**中创建和管理。

管理员账户有三种角色：

- `owner`：完全访问权限，包括管理员账户、保留策略、服务器 API Key、测试事件、日志导出和日志审核工作流
- `admin`：日志访问权限，加服务器 API Key 和测试事件管理；可标记、备注和归档日志
- `viewer`：只读访问日志页面、SSE、CSV 导出和 `GET /api/v1/logs`

生产环境部署请设置 `APP_ENV=production` 并使用至少 32 个字符的唯一 `SESSION_SECRET`。生产模式默认启用安全会话 Cookie；如果 TLS 在应用前端终止但 `APP_ENV` 不是 `production`，请显式设置 `SESSION_COOKIE_SECURE=true`。

### Cloudflare Turnstile

如需使用 Cloudflare Turnstile 保护登录表单，请在 Cloudflare 创建 Turnstile 组件并设置两个密钥：

```dotenv
TURNSTILE_SITE_KEY=0x...
TURNSTILE_SECRET_KEY=0x...
```

两个值都为空时 Turnstile 被禁用。如果只设置其中一个值，应用将拒绝启动。

## 时区配置

日志时间戳以绝对 `TIMESTAMPTZ` 值存储。页面、`datetime-local` 筛选器、仪表盘"今日"统计、CSV 时间戳和实时日志渲染都使用应用时区。默认为北京时间：

```dotenv
APP_TIME_ZONE=Asia/Shanghai
```

可使用任何有效的 IANA 时区名，如 `UTC`、`America/New_York` 或 `Europe/Berlin`。所有者也可从**系统设置**更改时区。

## 地理地图

**地理轨迹**页面在可平移/缩放地图上渲染 FiveM `coords`。如需使用真实 Los Santos 背景地图，请将您获得许可使用的地图图片放置在 `web/static/maps/los-santos.jpg`，或将 `GEO_MAP_IMAGE_URL` 指向另一个同源静态路径。默认投影使用常见 GTA V 世界边界：

```dotenv
GEO_MAP_IMAGE_URL=/static/maps/los-santos.jpg
GEO_MAP_MIN_X=-5610
GEO_MAP_MAX_X=6730
GEO_MAP_MIN_Y=-3850
GEO_MAP_MAX_Y=8350
```

如无图片，页面仍会显示带标记和轨迹线的可缩放坐标网格。

## AI JSON 解析

所有者可从日志详情对话框导入日志样本，或直接打开 **AI JSON**，请求 OpenAI 兼容模型一次性生成展示方法，然后保存该方法以便在日志详情中重复使用。无 AI 设置时，面板仍支持手动保存展示方法 JSON。

展示方法规范现在支持更丰富的布局：`summary_template`、`badges`、`metrics`、`fields`、`sections`、`lists`、`tables` 和 `json_blocks`。字段和列格式包括 `text`、`number`、`currency`、`delta`、`time`、`date`、`clock`、`percent`、`duration`、`coords`、`boolean`、`list` 和 `json`。

```dotenv
AI_JSON_BASE_URL=https://api.openai.com/v1
AI_JSON_API_KEY=
AI_JSON_MODEL=
```

## 技术栈

- **后端**：Go 1.25, PostgreSQL 17, HTMX
- **前端**：Go 模板，纯 JavaScript
- **容器**：Docker, Alpine Linux

## 许可证

VanceFiveMLog 采用 `AGPL-3.0-or-later` 许可证发布；详见 [LICENSE](LICENSE)。由于这是网络服务，修改后的部署必须让用户能够访问相应源代码。源代码地址和资产说明详见 [NOTICE.md](NOTICE.md)。

##版权说明
洛圣都地图来自于网络，如果有侵权请联系我，我会尽快删除。
