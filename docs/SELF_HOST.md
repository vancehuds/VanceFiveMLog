# 自有服务器部署教程

[English Version](SELF_HOST.en.md)

本文说明如何在自己的 Linux 服务器上部署 VanceFiveMLog。推荐使用 Docker Compose 运行应用和 PostgreSQL，并在前面放 Nginx、Caddy 或其他反向代理提供 HTTPS。

## 前置准备

1. 一台可公网访问的 Linux 服务器。
2. 已安装 Docker 和 Docker Compose 插件。
3. 一个域名，建议提前解析到服务器公网 IP。
4. 防火墙放行 `80`、`443`；如果不使用反向代理，也需要放行 `8080`。
5. 准备一个 32 位以上随机字符串作为 `SESSION_SECRET`，并准备一个强数据库密码。

## 1. 创建部署目录

```bash
sudo mkdir -p /opt/vancefivemlog
cd /opt/vancefivemlog
```

如果你使用 GitHub Actions 构建出的 GHCR 镜像，只需要在这个目录创建 `compose.yml` 和 `.env`。如果你想在服务器本地构建镜像，可以把仓库克隆到服务器，然后使用仓库根目录已有的 `Dockerfile`。

## 2. 配置环境变量

创建 `.env`：

```dotenv
APP_ENV=production
APP_ADDR=:8080
DATABASE_URL=postgres://vfl:请替换数据库密码@postgres:5432/vancefivemlog?sslmode=disable
SESSION_SECRET=请替换为至少32位随机字符串
INITIAL_ADMIN_USERNAME=admin
INITIAL_ADMIN_PASSWORD=请替换为强密码
RETENTION_DAYS=180
APP_TIME_ZONE=Asia/Shanghai
GEO_MAP_IMAGE_URL=/static/maps/los-santos.jpg
GEO_MAP_MIN_X=-5610
GEO_MAP_MAX_X=6730
GEO_MAP_MIN_Y=-3850
GEO_MAP_MAX_Y=8350
TURNSTILE_SITE_KEY=
TURNSTILE_SECRET_KEY=
AI_JSON_BASE_URL=https://api.openai.com/v1
AI_JSON_API_KEY=
AI_JSON_MODEL=
```

注意：

- `SESSION_SECRET` 在生产环境必须是唯一的 32 位以上随机字符串。
- `DATABASE_URL` 中的数据库密码要和下一步 `compose.yml` 里的 `POSTGRES_PASSWORD` 保持一致。
- `INITIAL_ADMIN_*` 只会在数据库没有管理员账号时创建首个管理员。已有管理员后，修改这两个变量不会重置密码。
- 如果 HTTPS 在反向代理层终止，保持 `APP_ENV=production` 即可，应用会给后台会话 Cookie 添加 `Secure` 标记。
- 如果启用 Cloudflare Turnstile，`TURNSTILE_SITE_KEY` 和 `TURNSTILE_SECRET_KEY` 必须同时填写。

## 3. 创建 Docker Compose 文件

### 方式 A：使用 GHCR 预构建镜像

创建 `compose.yml`：

```yaml
services:
  postgres:
    image: postgres:17-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: vancefivemlog
      POSTGRES_USER: vfl
      POSTGRES_PASSWORD: 请替换数据库密码
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U vfl -d vancefivemlog"]
      interval: 5s
      timeout: 5s
      retries: 10

  app:
    image: ghcr.io/<github-owner>/<repo>:main
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    env_file:
      - .env
    ports:
      - "127.0.0.1:8080:8080"

volumes:
  postgres_data:
```

把 `ghcr.io/<github-owner>/<repo>:main` 替换为你的实际镜像地址，例如 `ghcr.io/vancehuds/vancefivemlog:main`。如果镜像是私有的，需要先在服务器登录 GHCR：

```bash
echo "你的 GitHub Personal Access Token" | docker login ghcr.io -u 你的GitHub用户名 --password-stdin
```

### 方式 B：在服务器本地构建

如果你把仓库克隆到了服务器，可以在仓库根目录使用这个 `compose.yml`：

```yaml
services:
  postgres:
    image: postgres:17-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: vancefivemlog
      POSTGRES_USER: vfl
      POSTGRES_PASSWORD: 请替换数据库密码
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U vfl -d vancefivemlog"]
      interval: 5s
      timeout: 5s
      retries: 10

  app:
    build: .
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    env_file:
      - .env
    ports:
      - "127.0.0.1:8080:8080"

volumes:
  postgres_data:
```

## 4. 启动服务

```bash
docker compose up -d
docker compose logs -f app
```

如果看到应用监听 `:8080`，说明后端已启动。此时可以在服务器本机访问 `http://127.0.0.1:8080`。如果你临时开放了 `8080`，也可以通过 `http://服务器IP:8080` 测试。

生产环境建议不要直接暴露 `8080`，而是通过反向代理提供 HTTPS。

## 5. 配置反向代理

### Nginx 示例

```nginx
server {
    listen 80;
    server_name log.example.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_buffering off;
    }
}
```

为域名签发 HTTPS 证书后，把 FiveM Resource 的 endpoint 配置为 HTTPS 域名。

### Caddy 示例

```caddyfile
log.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

Caddy 会自动申请并续期 HTTPS 证书。

## 6. 配置 FiveM Resource

把 `fivem-resource` 复制到 FiveM 服务器资源目录，并把目录命名为 `vancefivemlog`，例如：

```text
resources/[logging]/vancefivemlog
```

推荐在 FiveM 的 `server.cfg` 使用仅服务端可见的 convar：

```cfg
set vfl_endpoint "https://log.example.com/api/v1/events"
set vfl_heartbeat_endpoint "https://log.example.com/api/v1/heartbeat"
set vfl_api_key "后台创建服务器时生成的一次性APIKey"

ensure vancefivemlog
```

然后重启 FiveM 服务器。后台 Dashboard 应该能看到 heartbeat 状态和实时日志。

## 7. 更新应用

如果使用 GHCR 镜像：

```bash
docker compose pull app
docker compose up -d
```

如果使用本地构建：

```bash
git pull
docker compose up -d --build
```

如果生产环境需要稳定可回滚，建议不要长期使用 `:main` 标签，而是使用 `v*` 版本标签或 `sha-...` 标签。

## 8. 备份和恢复

备份数据库：

```bash
docker compose exec postgres pg_dump -U vfl -d vancefivemlog > vancefivemlog.sql
```

恢复数据库前，请先停止应用服务，避免写入冲突：

```bash
docker compose stop app
cat vancefivemlog.sql | docker compose exec -T postgres psql -U vfl -d vancefivemlog
docker compose start app
```

同时建议定期备份 Docker volume 或保存数据库 dump 到服务器外部存储。

## 9. 排错

- **页面打不开**：检查 `docker compose ps`，确认 `app` 和 `postgres` 都是 Running/healthy；再检查防火墙、反向代理和域名解析。
- **启动时报数据库连接变量缺失**：确认 `.env` 中存在 `DATABASE_URL`，且 `env_file` 指向了正确的 `.env`。
- **数据库认证失败**：确认 `DATABASE_URL` 的密码和 `POSTGRES_PASSWORD` 一致。PostgreSQL volume 已创建后，修改 `POSTGRES_PASSWORD` 不会自动修改已有数据库用户密码。
- **登录失败或没有管理员**：确认第一次启动前已设置 `INITIAL_ADMIN_USERNAME` 和 `INITIAL_ADMIN_PASSWORD`。已有管理员后，请在后台“系统设置”管理账号。
- **FiveM 无法写入日志**：确认 endpoint 是公网 HTTPS 地址，API Key 没有多余空格，并查看 FiveM 控制台 HTTP 状态码。`status=0` 通常表示 DNS、TLS、防火墙或实际加载的 endpoint 配置有问题。
- **实时日志不刷新**：反向代理需要支持长连接/SSE。Nginx 示例中的 `proxy_buffering off` 不要省略。
