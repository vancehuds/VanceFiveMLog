# VanceFiveMLog

[![License: AGPL-3.0-or-later](https://img.shields.io/badge/License-AGPL--3.0--or--later-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.25-blue)](go.mod)

Go + PostgreSQL FiveM/Qbox log explorer with a dense HTMX-style admin UI, plugin API keys, SSE live logs, and a Qbox forwarding resource.

[中文说明](README.md) | English

## Features

- **Admin Management**: Login with signed HTTP-only session cookies, CSRF protection
- **Multi-Server Support**: Per-server API keys stored as SHA-256 hashes
- **Batch Ingestion**: Accept logs from FiveM/Qbox resources efficiently
- **Server Health**: Heartbeat tracking with online, stale, offline, and disabled status
- **Live Logs**: SSE live log stream on the dashboard
- **Dashboard Analytics**: 24-hour event trends and top event type rankings
- **Advanced Search**: Filter by server, event type, severity, resource, player, identifiers, message, metadata, review status, archive state, and time range
- **Review Workflow**: Log review with normal, suspicious, and violation states, plus administrator notes
- **AI JSON Parser**: Import log samples and generate display methods (owner-only feature)
- **CSV Export**: Export logs with review and archive fields
- **Time Zone Support**: Configurable display/query time zone (default: Asia/Shanghai)
- **Player Timeline**: Player behavior timeline visualization
- **Account Audit**: Account linkage audit trail
- **Interactive Map**: Los Santos coordinate map with pan, zoom, marker focus, and trail lines
- **Configurable Retention**: Default 180-day log retention
- **Event Bridge**: Built-in support for Qbox/QBCore, inventory, vehicle, and txAdmin events

## Quick Start

```bash
cp .env.example .env
docker compose up --build
```

Open `http://localhost:8080` and log in with the `INITIAL_ADMIN_*` values from `.env`.

## Deploy

- Zeabur: see [docs/ZEABUR.en.md](docs/ZEABUR.en.md)
- Plugin integration: see [docs/INTEGRATION.en.md](docs/INTEGRATION.en.md)

## FiveM Resource Setup

1. Open **System Settings** (系统设置)
2. Create a FiveM server and copy the one-time API key
3. Copy `fivem-resource` into your FiveM resources folder as `vancefivemlog`, e.g. `resources/[logging]/vancefivemlog`
4. Edit `vancefivemlog/config.lua` with the backend event URL, heartbeat URL, and API key
5. Add `ensure vancefivemlog` to `server.cfg`

## API Reference

### Event Ingestion

```http
POST /api/v1/events
Authorization: Bearer <server_api_key>
Content-Type: application/json
```

### Heartbeat

```http
POST /api/v1/heartbeat
Authorization: Bearer <server_api_key>
Content-Type: application/json
```

### Event Payload

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

Plugin-oriented events without the `events` wrapper:

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

For in-server integrations, use the FiveM export for automatic player identifier enrichment:

```lua
exports.vancefivemlog:Log('door forced', 'front bank door forced open', {
  severity = 'warning',
  source = source,
  metadata = { door_id = 'bank-front' }
})
```

## Local Development

Run PostgreSQL locally or with Docker, then:

```bash
go test ./...
go run ./cmd/server
```

After creating a server API key in the settings page, seed example events:

```bash
VFL_API_KEY=vfl_xxx go run ./cmd/seed
```

## Admin Security

All authenticated admin POST forms include CSRF protection. The first admin is created from `INITIAL_ADMIN_*` only when the database has no admin rows. Additional admins can be created and managed from **System Settings** by an `owner`.

Admin accounts use three roles:

- `owner`: full access, including admin accounts, retention policy, server API keys, test events, log export, and log review workflows
- `admin`: log access plus server API key and test event management; can mark, note, and archive logs
- `viewer`: read-only access to log pages, SSE, CSV export, and `GET /api/v1/logs`

For production deployments, set `APP_ENV=production` and use a unique `SESSION_SECRET` of at least 32 characters. Production mode enables secure session cookies by default; set `SESSION_COOKIE_SECURE=true` explicitly if TLS terminates in front of the app but `APP_ENV` is not `production`.

### Cloudflare Turnstile

To protect the login form with Cloudflare Turnstile, create a Turnstile widget in Cloudflare and set both keys:

```dotenv
TURNSTILE_SITE_KEY=0x...
TURNSTILE_SECRET_KEY=0x...
```

Turnstile is disabled when both values are empty. The app refuses to start if only one of the two values is set.

## Time Zone Configuration

Log timestamps are stored as absolute `TIMESTAMPTZ` values. Pages, `datetime-local` filters, dashboard "today" counts, CSV timestamps, and live log rendering use the application time zone. The default is Beijing time:

```dotenv
APP_TIME_ZONE=Asia/Shanghai
```

Use any valid IANA time zone name, such as `UTC`, `America/New_York`, or `Europe/Berlin`. Owners can also change the time zone from **System Settings**.

## Geo Map

The **Geo Trail** (地理轨迹) page renders FiveM `coords` on a pan/zoom map. To use a real Los Santos background, place a map image you are licensed to use at `web/static/maps/los-santos.jpg`, or point `GEO_MAP_IMAGE_URL` at another same-origin static path. The default projection uses common GTA V world bounds:

```dotenv
GEO_MAP_IMAGE_URL=/static/maps/los-santos.jpg
GEO_MAP_MIN_X=-5610
GEO_MAP_MAX_X=6730
GEO_MAP_MIN_Y=-3850
GEO_MAP_MAX_Y=8350
```

If no image is present, the page still shows a scalable coordinate grid with markers and trail lines.

## AI JSON Parsing

Owners can import a log sample from the log detail dialog or open **AI JSON** directly, ask an OpenAI-compatible model to generate a display method once, then save that method for reuse in log details. Without AI settings, the panel still supports manually saving display-method JSON.

Display-method specs support richer layouts: `summary_template`, `badges`, `metrics`, `fields`, `sections`, `lists`, `tables`, and `json_blocks`. Field and column formats include `text`, `number`, `currency`, `delta`, `time`, `date`, `clock`, `percent`, `duration`, `coords`, `boolean`, `list`, and `json`.

```dotenv
AI_JSON_BASE_URL=https://api.openai.com/v1
AI_JSON_API_KEY=
AI_JSON_MODEL=
```

## Tech Stack

- **Backend**: Go 1.25, PostgreSQL 17, HTMX
- **Frontend**: Go templates, vanilla JavaScript
- **Container**: Docker, Alpine Linux

## License

VanceFiveMLog is released under `AGPL-3.0-or-later`; see [LICENSE](LICENSE). Because this is a network service, modified deployments must provide their users access to the corresponding source code. See [NOTICE.en.md](NOTICE.en.md) for source URL and asset notes.

## Copyright Notice
Los Santos map is sourced from the internet. If there is any infringement, please contact me and I will delete it as soon as possible.
