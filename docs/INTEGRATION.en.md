# VanceFiveMLog Integration Guide

This document is for developers who need to integrate other FiveM plugins, Qbox/QBCore resources, or external services with VanceFiveMLog.

## 1. Choosing an Integration Method

Recommended priority:

1. **FiveM server resource integration**: Use `exports.vancefivemlog:Log(...)` inside your FiveM server resource.
2. **Event bridge configuration**: Configure event bridging in `fivem-resource/config.lua`'s `Config.EventBridge`.
3. **External systems**: Make direct HTTP requests to `POST /api/v1/events`.

The FiveM resource integration is recommended because `vancefivemlog` automatically enriches player names, license, discord, steam, citizenid, and coordinates from the `source` (server id).

## 2. Prerequisites

First, ensure the log resource is installed and started:

```cfg
ensure vancefivemlog
ensure your_resource
```

The FiveM export name comes from the resource name. Copy/rename this project's `fivem-resource` folder to `vancefivemlog`, e.g. `resources/[logging]/vancefivemlog`. If the directory is still named `fivem-resource` or something else, `exports.vancefivemlog:Log(...)` may not find the export.

If your resource will call the export, declare the dependency in your `fxmanifest.lua`:

```lua
dependency 'vancefivemlog'
```

**Recommended**: Use server-only convars in `server.cfg` to avoid hardcoding the backend URL and API Key:

```cfg
set vfl_endpoint "https://your-backend-domain/api/v1/events"
set vfl_heartbeat_endpoint "https://your-backend-domain/api/v1/heartbeat"
set vfl_api_key "APIKey-generated-in-admin-panel-System-Settings"
```

Alternatively, edit `vancefivemlog/config.lua`:

```lua
Config.Endpoint = 'https://your-backend-domain/api/v1/events'
Config.HeartbeatEndpoint = 'https://your-backend-domain/api/v1/heartbeat'
Config.APIKey = 'APIKey-generated-in-admin-panel-System-Settings'
```

## 3. Recommended: FiveM Export

Basic usage:

```lua
exports.vancefivemlog:Log('inventory_remove', 'removed marked bills', {
  severity = 'warning',
  source = source,
  metadata = {
    item = 'markedbills',
    amount = 5,
    reason = 'police evidence'
  }
})
```

Function signature:

```lua
exports.vancefivemlog:Log(eventType, message, options)
```

### Safe Wrapper Pattern

When modifying third-party plugins, use a local helper so the original plugin won't break if the log resource is not started or the export call fails:

```lua
local vflMissingWarned = false

local function vflLog(eventType, message, options)
  if type(message) == 'table' and options == nil then
    options = message
    message = options.message
  elseif type(options) ~= 'table' then
    options = { metadata = options }
  end
  options.resource = options.resource or GetCurrentResourceName()

  if GetResourceState('vancefivemlog') == 'started' then
    local ok, result = pcall(function()
      return exports.vancefivemlog:Log(eventType, message, options)
    end)
    if ok then return result end
    print(('[VanceFiveMLog] export Log failed: %s'):format(tostring(result)))
  elseif not vflMissingWarned then
    vflMissingWarned = true
    print('[VanceFiveMLog] logger resource is not started; using local fallback event')
  end

  TriggerEvent('vancefivemlog:server:Log', eventType, message, options)
  return true
end
```

### Parameter Reference

- `eventType`: Event type identifier, use stable lowercase English with underscores, e.g. `inventory_remove`, `money_change`, `admin_ban`.
- `message`: Short description displayed in log list.
- `options.severity`: Severity level: `info`, `success`, `warning`, `error`.
- `options.source`: FiveM player server id. When provided, player info and coordinates are auto-enriched.
- `options.metadata`: Business fields written to backend `metadata`.
- `options.resource`: Override resource name; defaults to the calling resource name.
- `options.coords`: Manual coordinates in format `{ x = 1.0, y = 2.0, z = 3.0 }`.
- `options.occurred_at`: Custom event time using ISO/RFC3339 string.

`message` can also be omitted by passing the config table as the second argument:

```lua
exports.vancefivemlog:Log('money_change', {
  severity = 'warning',
  source = source,
  message = 'cash changed',
  metadata = {
    account = 'cash',
    amount = 2500,
    operation = 'remove'
  }
})
```

Or pass a complete event table:

```lua
exports.vancefivemlog:Log({
  event_type = 'vehicle_spawn',
  severity = 'info',
  source = source,
  message = 'vehicle spawned',
  metadata = {
    model = 'sultan',
    plate = 'VANCE123'
  }
})
```

## 4. Field Compatibility

For third-party plugin compatibility, both the resource and backend accept these field aliases:

| Preferred Field | Compatible Fields | Description |
| --- | --- | --- |
| `event_type` | `type`, `event` | Event type |
| `severity` | `level` | Severity level |
| `source` | `player_source`, `player` | FiveM player server id |
| `resource` | `plugin`, `plugin_resource` | Plugin or resource name |
| `metadata` | `data` | Business附加数据 |

If event type contains spaces, the backend normalizes it to underscores, e.g. `door forced` becomes `door_forced`.

## 5. Common Plugin Examples

### Inventory Changes

```lua
RegisterNetEvent('my_inventory:server:removeItem', function(item, count, reason)
  local src = source

  exports.vancefivemlog:Log('inventory_remove', 'item removed', {
    severity = 'warning',
    source = src,
    metadata = {
      item = item,
      count = count,
      reason = reason
    }
  })
end)
```

### Money Changes

```lua
local function logMoneyChange(src, account, amount, operation, reason)
  exports.vancefivemlog:Log('money_change', 'player money changed', {
    severity = amount >= 100000 and 'warning' or 'info',
    source = src,
    metadata = {
      account = account,
      amount = amount,
      operation = operation,
      reason = reason
    }
  })
end
```

### Admin Actions

```lua
exports.vancefivemlog:Log('admin_kick', 'admin kicked player', {
  severity = 'warning',
  source = target,
  metadata = {
    admin = GetPlayerName(source),
    target = target,
    reason = reason
  }
})
```

### Immediate Flush

By default, the resource batches reports per `Config.FlushIntervalMs`. For critical events needing immediate write:

```lua
exports.vancefivemlog:Log('admin_ban', 'player banned', {
  severity = 'error',
  source = target,
  metadata = { reason = reason }
})

exports.vancefivemlog:Flush()
```

Do NOT call `Flush()` on high-frequency events, as it increases HTTP request volume.

## 6. Event Bridge Integration

If you don't want to modify third-party plugin source code, add `Config.EventBridge` entries in `fivem-resource/config.lua`.

Example:

```lua
{
  event = 'my_inventory:server:removeItem',
  enabled = true,
  event_type = 'inventory_remove',
  category = 'items',
  severity = 'warning',
  source_arg = 'source',
  message = 'inventory item removed',
  metadata = {
    item = 1,
    count = 2,
    reason = 3
  }
}
```

Fields:

- `event`: FiveM server event name to listen for.
- `event_type`: Event type written to the log system.
- `category`: Corresponds to `Config.Events` switches.
- `severity`: Severity level.
- `source_arg`: Player source, can be `source`, parameter index, or table field path.
- `metadata`: Business fields extracted from event parameters.

## 7. Direct HTTP Integration

External services can make direct requests:

```http
POST /api/v1/events
Authorization: Bearer <server_api_key>
Content-Type: application/json
```

Single event:

```json
{
  "event": "door forced",
  "level": "warning",
  "player_source": 12,
  "plugin_resource": "doorlocks",
  "message": "front bank door forced open",
  "data": {
    "door_id": "bank-front"
  }
}
```

Batch events:

```json
[
  {
    "type": "money add",
    "plugin": "banking",
    "player_source": 12,
    "message": "cash added",
    "data": { "amount": 500 }
  },
  {
    "type": "money remove",
    "plugin": "banking",
    "player_source": 20,
    "message": "cash removed",
    "data": { "amount": 100 }
  }
]
```

Also compatible with raw wrapper format:

```json
{
  "events": [
    {
      "event_type": "inventory_remove",
      "severity": "warning",
      "source": 12,
      "resource": "inventory",
      "message": "removed marked bills",
      "metadata": {
        "item": "markedbills",
        "amount": 5
      }
    }
  ]
}
```

The backend accepts a maximum of 500 events per request. Batch or throttle high-frequency logs.

## 8. Field Conventions

### Event Naming

- Use lowercase English with underscores, e.g. `inventory_remove`.
- Keep same-type events under stable names; don't put dynamic values in `event_type`.
- Put dynamic values in `metadata`, such as item names, amounts, plates, reasons.

### Severity Levels

- `info`: Normal behavior, e.g. entering areas, ordinary item changes.
- `success`: Critical flows completed successfully, e.g. transactions.
- `warning`: Behaviors needing attention, e.g. large money changes, item deletions, admin warnings.
- `error`: High-risk or failed events, e.g. bans, anti-cheat hits, abnormal failures.

### Privacy and Performance

- Do NOT write sensitive info like passwords, tokens, bank card numbers to `metadata`.
- High-frequency events should be aggregated before reporting; avoid logging every frame or tick.
- Keep `message` short; detailed context goes in `metadata`.

## 9. Troubleshooting

- **FiveM console shows `unauthorized`**: Check if `Config.APIKey` is the current server's API Key from admin panel and has no extra spaces.
- **FiveM console shows `status=0`**: HTTP request didn't get a response, usually DNS, TLS, firewall issues, or the actual loaded endpoint is not a public address. Check the endpoint and `transport failed ... error=...` printed at resource startup.
- **Export not found error**: Confirm the log resource directory is named `vancefivemlog`, `server.cfg` has `ensure vancefivemlog` before starting plugins that call it; modified plugins should call the export in server-side Lua only, not client scripts.
- **Log shows no resource name**: Confirm using `exports.vancefivemlog:Log(...)` or manually passing `resource`/`plugin_resource`.
- **Player info empty**: Confirm passing the server player `source`, not license, identifier, or client id.
- **Backend returns `invalid log event`**: Confirm at least one of `event_type`, `type`, or `event` is provided.
- **Coordinates empty**: Coordinates may be empty if player ped doesn't exist, player just disconnected, or event is not a player event.
- **High-frequency events lost**: Check `Config.MaxQueue`, `Config.BatchSize`, backend network connectivity, and FiveM console HTTP status codes.
