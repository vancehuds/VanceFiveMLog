# VanceFiveMLog FiveM Resource

Full plugin integration guide: [../docs/INTEGRATION.md](../docs/INTEGRATION.md).

1. Copy this folder to your FiveM `resources` directory as `vancefivemlog`, for example `resources/[logging]/vancefivemlog`.
2. Create a server in the web admin settings page and copy the one-time API key.
3. Configure the endpoint and API key. Prefer server-only convars in `server.cfg` so the API key is not stored in the resource folder:

```cfg
set vfl_endpoint "https://your-domain.example/api/v1/events"
set vfl_heartbeat_endpoint "https://your-domain.example/api/v1/heartbeat"
set vfl_api_key "vfl_your_generated_key"
```

You can still set `Config.Endpoint`, `Config.HeartbeatEndpoint`, and `Config.APIKey` in `config.lua`; convars override those values when present.

## License

This resource is distributed with VanceFiveMLog under `AGPL-3.0-or-later`. See the repository `LICENSE`.

4. Add this to `server.cfg` after the `set` lines:

```cfg
ensure vancefivemlog
```

On startup the resource prints the effective endpoints and a redacted API key. If FiveM reports `status=0`, check the printed transport error first; it usually points to DNS, TLS, firewall, or an endpoint that the FXServer host cannot reach.

The folder name matters for exports: integrated resources call `exports.vancefivemlog:*`, so the installed resource should be named `vancefivemlog`. The manifest also declares `provide 'vancefivemlog'` as a compatibility alias, but keeping the folder name canonical avoids startup-order and older artifact issues.

Other resources can log custom events with the plugin-friendly export:

```lua
exports.vancefivemlog:Log('inventory_remove', 'removed marked bills', {
  severity = 'warning',
  source = source,
  metadata = { item = 'markedbills', amount = 5 }
})
```

For optional integrations, wrap the export so the target plugin keeps running and failed calls are visible in the FiveM console:

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

`Log` automatically tags the event with the invoking resource name and enriches player name, identifiers, citizen ID, and coordinates when `source` is provided.
When qbx_core is available it also adds `character_name`, `job`, and `gang` to metadata, matching the audit context used by VanceLogger.

The lower-level export still accepts the backend event shape directly:

```lua
exports.vancefivemlog:LogEvent({
  event_type = 'inventory_remove',
  severity = 'warning',
  source = source,
  message = 'removed marked bills',
  metadata = { item = 'markedbills', amount = 5 }
})
```

The resource sends a heartbeat every `Config.HeartbeatIntervalMs` so the dashboard can show whether the plugin is online even when no player event is currently being written.

## Built-in audit events

The resource now includes VanceLogger-style structured audit capture:

- `QBCore:Server:OnMoneyChange` writes `money_change` with `money_type`, `amount`, `operation`, `reason`, and current `balance`.
- `ox_inventory` hooks write `inventory_diff` with item deltas, before/after counts, metadata, and hook context.
- Player events are enriched with role context where possible: `character_name`, `job`, `gang`, identifiers, and coordinates.

Tune these with `Config.Events.money`, `Config.Events.items`, `Config.MoneyTypes`, and `Config.Inventory` in `config.lua`.
The legacy EventBridge money entry is disabled by default to avoid duplicate `money_change` events.

## Event bridge

`config.lua` includes `Config.EventBridge` entries for common Qbox/QBCore, ox_inventory, vehicle key, and txAdmin events. Keep the entries that match your server resources and rename the event or argument mapping when your resources use custom event names.

Each bridge entry can set:

- `event`: FiveM server event name to listen for.
- `event_type`: normalized type shown in the web UI.
- `category`: one of the `Config.Events` toggles.
- `source_arg`: argument index, dotted table path, or `source`.
- `metadata`: map of metadata fields to argument indexes or dotted paths.
