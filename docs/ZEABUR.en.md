# Zeabur Deployment Guide

This project supports Zeabur deployment. Zeabur will detect the `Dockerfile` in the repository root to build the Go service, and the application will automatically read the `PORT` injected by Zeabur at startup.

## Prerequisites

1. Push this project to GitHub, GitLab, or a Git repository supported by Zeabur.
2. Ensure the repository root contains `Dockerfile`.
3. Do not upload `.env`; production environment variables are configured in the Zeabur console.

## 1. Create a Zeabur Project

1. Log in to the Zeabur console.
2. Create a new Project.
3. First, add a PostgreSQL service and wait for the database to initialize.
4. Then add a Git service and select this project repository.
5. Choose Dockerfile as the build method. If Zeabur auto-detects the root `Dockerfile`, keep the default.

## 2. Configure Environment Variables

Configure in the Git service's Variables:

```env
APP_ENV=production
SESSION_SECRET=replace-with-at-least-32-random-characters
INITIAL_ADMIN_USERNAME=admin
INITIAL_ADMIN_PASSWORD=replace-with-strong-password
RETENTION_DAYS=180
APP_TIME_ZONE=Asia/Shanghai
TURNSTILE_SITE_KEY=
TURNSTILE_SECRET_KEY=
```

Database connection: choose one of the following:

- **Recommended**: Bind or copy the PostgreSQL service's connection string as `DATABASE_URL`.
- **Compatibility**: If your Zeabur PostgreSQL service has `POSTGRES_CONNECTION_STRING` or `POSTGRES_URI` injected, the app will read them automatically.

**Important Notes:**

- Do not set `APP_ADDR` in Zeabur; let the app use the `PORT` injected by Zeabur.
- `APP_ENV=production` enables stricter session key validation and adds `Secure` flag to admin session cookies by default.
- `APP_TIME_ZONE` uses IANA timezone names; default is Beijing time `Asia/Shanghai`. You can also change it from **System Settings** in the admin panel.
- Configure both `TURNSTILE_SITE_KEY` and `TURNSTILE_SECRET_KEY` to enable Cloudflare Turnstile on the login page; both must be either empty or filled.
- If the connection string includes SSL parameters, keep Zeabur's original values.
- `INITIAL_ADMIN_*` only creates the first admin when the database has no admin rows. If admins already exist, changing these variables will not reset passwords.

## 3. Bind Domain and Access Admin Panel

1. In the Git service's Domains, generate a Zeabur default domain, or bind your own domain.
2. Open the domain and log in with `INITIAL_ADMIN_USERNAME` and `INITIAL_ADMIN_PASSWORD`.
3. Go to **System Settings**, create a FiveM server, and copy the one-time API Key.

## 4. Configure FiveM Resource

Copy `fivem-resource` to your FiveM server's resource directory and rename it to `vancefivemlog`, for example:

```
resources/[logging]/vancefivemlog
```

**Recommended**: Use server-only convars in `server.cfg` to avoid hardcoding the API Key in the resource directory:

```cfg
set vfl_endpoint "https://your-zeabur-domain/api/v1/events"
set vfl_heartbeat_endpoint "https://your-zeabur-domain/api/v1/heartbeat"
set vfl_api_key "one-time-APIKey-generated-in-admin-panel"
```

Alternatively, edit `fivem-resource/config.lua` directly:

```lua
Config.Endpoint = 'https://your-zeabur-domain/api/v1/events'
Config.HeartbeatEndpoint = 'https://your-zeabur-domain/api/v1/heartbeat'
Config.APIKey = 'one-time-APIKey-generated-in-admin-panel'
```

Add to `server.cfg`, after the `set` configurations above:

```cfg
ensure vancefivemlog
```

After restarting the FiveM server, you should see heartbeat status and live logs in the admin Dashboard.

## 5. Troubleshooting

- **Page won't open**: Check if the Git service built successfully and is in Running state; confirm you didn't manually set an incorrect `APP_ADDR`.
- **Database connection error on startup**: Configure `DATABASE_URL` for the Git service, or confirm that PostgreSQL service variables are correctly bound to the Git service.
- **Login failed or no admin exists**: Confirm `INITIAL_ADMIN_USERNAME` and `INITIAL_ADMIN_PASSWORD` were set on first startup. If the database already has admins, manage accounts from **System Settings** in the admin panel.
- **FiveM cannot write logs**: Confirm the endpoint uses a public HTTPS domain, the API Key has no extra spaces, and check HTTP status codes in the FiveM console. `status=0` means FXServer didn't receive an HTTP response, usually due to DNS, TLS, firewall issues, or the actual loaded endpoint configuration problem.

## 6. Official References

- [Zeabur Deploy Go App](https://zeabur.com/docs/en-US/guides/go)
- [Zeabur Deploying with Dockerfile](https://zeabur.com/docs/en-US/deploy/methods/dockerfile)
