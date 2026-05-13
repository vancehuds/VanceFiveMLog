# Self-Hosted Deployment Guide

[中文说明](SELF_HOST.md)

This guide explains how to deploy VanceFiveMLog on your own Linux server. The recommended setup is Docker Compose for the app and PostgreSQL, with Nginx, Caddy, or another reverse proxy in front to provide HTTPS.

## Prerequisites

1. A publicly reachable Linux server.
2. Docker and the Docker Compose plugin installed.
3. A domain name pointed to the server's public IP.
4. Firewall rules allowing `80` and `443`; if you skip the reverse proxy, also allow `8080`.
5. A random `SESSION_SECRET` of at least 32 characters and a strong database password.

## 1. Create a Deployment Directory

```bash
sudo mkdir -p /opt/vancefivemlog
cd /opt/vancefivemlog
```

If you use the GHCR image built by GitHub Actions, this directory only needs `compose.yml` and `.env`. If you want to build the image on the server, clone the repository and use the root `Dockerfile`.

## 2. Configure Environment Variables

Create `.env`:

```dotenv
APP_ENV=production
APP_ADDR=:8080
DATABASE_URL=postgres://vfl:replace-database-password@postgres:5432/vancefivemlog?sslmode=disable
SESSION_SECRET=replace-with-at-least-32-random-characters
INITIAL_ADMIN_USERNAME=admin
INITIAL_ADMIN_PASSWORD=replace-with-strong-password
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

Notes:

- `SESSION_SECRET` must be unique and at least 32 characters in production.
- The database password in `DATABASE_URL` must match `POSTGRES_PASSWORD` in `compose.yml`.
- `INITIAL_ADMIN_*` creates the first admin only when the database has no admin rows. If admins already exist, changing these variables will not reset passwords.
- Keep `APP_ENV=production` when HTTPS terminates at the reverse proxy. The app will add the `Secure` flag to admin session cookies.
- If you enable Cloudflare Turnstile, `TURNSTILE_SITE_KEY` and `TURNSTILE_SECRET_KEY` must be set together.

## 3. Create the Docker Compose File

### Option A: Use the GHCR Prebuilt Image

Create `compose.yml`:

```yaml
services:
  postgres:
    image: postgres:17-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: vancefivemlog
      POSTGRES_USER: vfl
      POSTGRES_PASSWORD: replace-database-password
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

Replace `ghcr.io/<github-owner>/<repo>:main` with your real image path, for example `ghcr.io/vancehuds/vancefivemlog:main`. If the image is private, log in to GHCR on the server first:

```bash
echo "your GitHub Personal Access Token" | docker login ghcr.io -u your-github-username --password-stdin
```

### Option B: Build on the Server

If you cloned the repository to the server, use this `compose.yml` from the repository root:

```yaml
services:
  postgres:
    image: postgres:17-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: vancefivemlog
      POSTGRES_USER: vfl
      POSTGRES_PASSWORD: replace-database-password
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

## 4. Start the Services

```bash
docker compose up -d
docker compose logs -f app
```

If the app is listening on `:8080`, the backend is running. You can test it from the server with `http://127.0.0.1:8080`. If you temporarily expose `8080`, you can also test `http://server-ip:8080`.

For production, avoid exposing `8080` directly. Put the app behind an HTTPS reverse proxy.

## 5. Configure a Reverse Proxy

### Nginx Example

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

After issuing an HTTPS certificate for the domain, configure the FiveM Resource endpoints with the HTTPS domain.

### Caddy Example

```caddyfile
log.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

Caddy automatically issues and renews HTTPS certificates.

## 6. Configure the FiveM Resource

Copy `fivem-resource` to your FiveM server's resources folder and rename it to `vancefivemlog`, for example:

```text
resources/[logging]/vancefivemlog
```

Recommended: use server-only convars in `server.cfg`:

```cfg
set vfl_endpoint "https://log.example.com/api/v1/events"
set vfl_heartbeat_endpoint "https://log.example.com/api/v1/heartbeat"
set vfl_api_key "one-time-APIKey-generated-in-admin-panel"

ensure vancefivemlog
```

Restart the FiveM server. The admin Dashboard should show heartbeat status and live logs.

## 7. Update the App

If you use the GHCR image:

```bash
docker compose pull app
docker compose up -d
```

If you build locally:

```bash
git pull
docker compose up -d --build
```

For stable and rollback-friendly production deployments, prefer a `v*` version tag or a `sha-...` tag instead of using `:main` long term.

## 8. Backup and Restore

Back up the database:

```bash
docker compose exec postgres pg_dump -U vfl -d vancefivemlog > vancefivemlog.sql
```

Before restoring, stop the app to avoid write conflicts:

```bash
docker compose stop app
cat vancefivemlog.sql | docker compose exec -T postgres psql -U vfl -d vancefivemlog
docker compose start app
```

Also consider periodically backing up the Docker volume or saving database dumps to storage outside the server.

## 9. Troubleshooting

- **Page won't open**: Check `docker compose ps` and confirm `app` and `postgres` are Running/healthy, then check the firewall, reverse proxy, and DNS.
- **Database connection variable is missing on startup**: Confirm `.env` contains `DATABASE_URL` and `env_file` points to the correct `.env`.
- **Database authentication failed**: Confirm the password in `DATABASE_URL` matches `POSTGRES_PASSWORD`. After the PostgreSQL volume has been created, changing `POSTGRES_PASSWORD` does not automatically update the existing database user password.
- **Login failed or no admin exists**: Confirm `INITIAL_ADMIN_USERNAME` and `INITIAL_ADMIN_PASSWORD` were set before the first startup. If admins already exist, manage accounts from **System Settings**.
- **FiveM cannot write logs**: Confirm the endpoint is a public HTTPS URL, the API Key has no extra spaces, and check HTTP status codes in the FiveM console. `status=0` usually means DNS, TLS, firewall, or the actually loaded endpoint configuration is wrong.
- **Live logs do not update**: The reverse proxy must support long-lived connections/SSE. Do not omit `proxy_buffering off` from the Nginx example.
