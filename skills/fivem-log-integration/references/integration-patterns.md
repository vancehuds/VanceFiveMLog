# VanceFiveMLog Integration Patterns

## API Surface

Preferred server-side export:

```lua
exports.vancefivemlog:Log('event_type', 'short message', {
  severity = 'info',
  source = source,
  metadata = {}
})
```

Other exports:

```lua
exports.vancefivemlog:LogEvent({
  event_type = 'inventory_remove',
  severity = 'warning',
  source = source,
  message = 'item removed',
  metadata = { item = item, count = count }
})

exports.vancefivemlog:Flush()
```

Accepted field aliases:

| Canonical | Aliases |
| --- | --- |
| `event_type` | `type`, `event` |
| `severity` | `level` |
| `source` | `player_source`, `player` |
| `resource` | `plugin_resource`, `plugin` |
| `metadata` | `data` |

## Manifest And Server Config

Required startup order:

```cfg
set vfl_endpoint "https://your-domain.example/api/v1/events"
set vfl_heartbeat_endpoint "https://your-domain.example/api/v1/heartbeat"
set vfl_api_key "vfl_generated_key"

ensure vancefivemlog
ensure target_resource
```

`exports.vancefivemlog:*` resolves by FiveM resource name. Install or rename the logger folder as `vancefivemlog`; copying it as `fivem-resource` means direct export calls will not find it on older artifact behavior.

For direct exports, either make the dependency explicit:

```lua
dependency 'vancefivemlog'
```

Or use an optional helper and omit the dependency:

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

## Common Direct Export Snippets

Inventory removal:

```lua
vflLog('inventory_remove', 'item removed', {
  severity = 'warning',
  source = src,
  metadata = {
    item = item,
    count = count,
    reason = reason
  }
})
```

Money change:

```lua
vflLog('money_change', 'player money changed', {
  severity = amount >= 100000 and 'warning' or 'info',
  source = src,
  metadata = {
    account = account,
    amount = amount,
    operation = operation,
    reason = reason
  }
})
```

Admin action:

```lua
vflLog('admin_kick', 'admin kicked player', {
  severity = 'warning',
  source = target,
  metadata = {
    admin_source = source,
    admin_name = GetPlayerName(source),
    target = target,
    reason = reason
  }
})
```

Vehicle event:

```lua
vflLog('vehicle_spawn', 'vehicle spawned', {
  severity = 'info',
  source = src,
  metadata = {
    model = model,
    plate = plate
  }
})
```

Critical event with immediate flush:

```lua
vflLog('admin_ban', 'player banned', {
  severity = 'error',
  source = target,
  metadata = { reason = reason, admin_source = source }
})
exports.vancefivemlog:Flush()
```

## EventBridge Snippet

Add entries to `fivem-resource/config.lua` when not editing the target plugin:

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

`source_arg` may be `source`, an argument number, or a dotted table path such as `target.id`.

## Severity Guide

Use `info` for normal activity, `success` for successful important workflows, `warning` for actions that need review, and `error` for failed, blocked, banned, exploit, or anti-cheat events.

## Troubleshooting

- `unauthorized`: API key is wrong, stale, missing, or has extra whitespace.
- `status=0`: FXServer cannot reach the endpoint, DNS/TLS/firewall failed, or the endpoint is local to the wrong machine.
- Missing player context: pass a FiveM server `source`, not a license or client id.
- Missing resource name: prefer direct exports or set `resource`/`plugin_resource`.
- Repeated logs: disable overlapping built-in audit events or duplicate EventBridge entries.
