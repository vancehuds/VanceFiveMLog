---
name: fivem-log-integration
description: Add VanceFiveMLog logging to FiveM, Qbox, or QBCore resources. Use when Codex needs to connect a FiveM plugin/resource to this repository's log collector, add exports.vancefivemlog:Log calls, configure Config.EventBridge mappings, patch fxmanifest.lua dependencies, map plugin events into normalized log payloads, or troubleshoot missing FiveM log ingestion.
---

# FiveM Log Integration

## Overview

Use this skill to make an agent integrate a FiveM resource with VanceFiveMLog in one pass: inspect the target resource, choose the safest integration path, patch Lua/manifest/config files, and verify the resulting log flow.

Load `references/integration-patterns.md` before editing Lua or EventBridge entries. Run `scripts/scan_fivem_resource.py` on the target resource when a resource path is available.

## Workflow

1. Identify the target resource directory and whether the user allows source changes.
2. Run the scanner:

   ```bash
   python3 skills/fivem-log-integration/scripts/scan_fivem_resource.py /path/to/fivem-resource
   ```

3. Choose the integration path:
   - Direct export: preferred when editing the target resource is allowed.
   - EventBridge: use when the target resource should remain untouched and it emits server events.
   - Direct HTTP: use only for external services or code that cannot call FiveM exports.
4. Patch only server-side Lua. Do not put API keys or backend URLs in target plugin source.
5. Add or confirm resource startup order:

   ```cfg
   ensure vancefivemlog
   ensure target_resource
   ```

   The VanceFiveMLog resource directory must be named `vancefivemlog` for `exports.vancefivemlog:*` calls to resolve. If a server already copied it as `fivem-resource` or another folder name, either rename the folder or use the fallback event helper below.

6. Verify with a syntax check when available, the scanner output, and any repo tests relevant to changed code.

## Direct Export Path

Use `exports.vancefivemlog:Log(eventType, message, options)` inside server handlers. Prefer a small local helper when adding more than one call site, especially in third-party resources that should keep running if the logger is absent:

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

  -- Fallback for servers that installed the logger under a nonstandard folder name.
  TriggerEvent('vancefivemlog:server:Log', eventType, message, options)
  return true
end
```

When the target resource must require the logger, add this to `fxmanifest.lua`:

```lua
dependency 'vancefivemlog'
```

Pass the player server id as `options.source` so VanceFiveMLog can enrich player name, identifiers, citizenid, role metadata, and coordinates. Put actor/admin/target details in `metadata` when an event involves more than one player.

## EventBridge Path

Use `fivem-resource/config.lua` `Config.EventBridge` when you should not edit the target plugin. Add bridge entries for server events that already fire with useful arguments. Map `source_arg` to `source`, an argument index, or a dotted table path.

Bridge entries are best for coarse events. Direct exports are better when logging depends on local variables, branch outcomes, or computed context that is not present in emitted event arguments.

## Payload Rules

- Use stable lower snake_case `event_type` values such as `inventory_remove`, `money_change`, `admin_ban`, or `vehicle_spawn`.
- Use severities `info`, `success`, `warning`, and `error`.
- Keep `message` short and human-readable.
- Put dynamic data in `metadata`, not in `event_type`.
- Never log passwords, tokens, API keys, payment data, or private secrets.
- Avoid per-frame/per-tick logging. Batch or throttle high-frequency events.
- Call `exports.vancefivemlog:Flush()` only after critical low-volume events such as bans or anti-cheat hits.

## Verification

After editing:

1. Re-run the scanner and confirm it reports VanceFiveMLog calls or the expected bridge path.
2. Check Lua syntax with `luac -p` when `luac` is installed.
3. If the repo has tests, run the narrowest relevant command, or `go test ./...` when this repository's Go code changed.
4. Tell the user which files were changed and whether any runtime setup remains, such as creating a server API key in the web UI.
