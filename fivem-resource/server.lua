local queue = {}
local queueHead = 1
local queueTail = 0
local flushing = false
local heartbeatInFlight = false
local inventorySnapshots = {}
local inventoryPending = {}
local inventoryContext = {}
local inventoryHooks = {}
local inventoryStarted = false
local exportedAlias = 'vancefivemlog'

local function trim(value)
  if value == nil then return '' end
  return tostring(value):match('^%s*(.-)%s*$') or ''
end

local function configValue(convar, fallback)
  local value = trim(GetConvar(convar, ''))
  if value ~= '' then return value end
  return trim(fallback)
end

local function deriveHeartbeatEndpoint(endpoint)
  if endpoint == '' then return '' end
  local derived, replacements = endpoint:gsub('/events$', '/heartbeat', 1)
  if replacements > 0 then return derived end
  return ''
end

Config.Endpoint = configValue('vfl_endpoint', Config.Endpoint)
Config.HeartbeatEndpoint = configValue('vfl_heartbeat_endpoint', Config.HeartbeatEndpoint)
Config.APIKey = configValue('vfl_api_key', Config.APIKey)

if Config.HeartbeatEndpoint == '' then
  Config.HeartbeatEndpoint = deriveHeartbeatEndpoint(Config.Endpoint)
end

local function redact(value)
  value = trim(value)
  if value == '' then return '<empty>' end
  if #value <= 12 then return '<redacted>' end
  return value:sub(1, 4) .. '...' .. value:sub(-4)
end

local function printConfigStatus()
  local resourceName = GetCurrentResourceName()
  print(('[VanceFiveMLog] endpoint=%s heartbeat=%s api_key=%s'):format(
    Config.Endpoint ~= '' and Config.Endpoint or '<empty>',
    Config.HeartbeatEndpoint ~= '' and Config.HeartbeatEndpoint or '<empty>',
    redact(Config.APIKey)
  ))

  if resourceName ~= exportedAlias then
    print(('[VanceFiveMLog] resource name is %s; fxmanifest provide %s keeps exports.%s available for integrated resources'):format(
      resourceName,
      exportedAlias,
      exportedAlias
    ))
  end

  if Config.Endpoint:find('^https?://') ~= 1 then
    print('[VanceFiveMLog] config warning: Config.Endpoint must start with http:// or https://')
  end
  if Config.HeartbeatEndpoint ~= '' and Config.HeartbeatEndpoint:find('^https?://') ~= 1 then
    print('[VanceFiveMLog] config warning: Config.HeartbeatEndpoint must start with http:// or https://')
  end
  if Config.Endpoint:find('^https?://127%.0%.0%.1') or Config.Endpoint:find('^https?://localhost') then
    print('[VanceFiveMLog] config warning: localhost endpoints only work when the web app runs on the same machine as FXServer')
  end
  if Config.APIKey == '' or Config.APIKey == 'replace-with-generated-api-key' then
    print('[VanceFiveMLog] config warning: API key is not configured')
  end
end

local function requestHeaders()
  return {
    ['Content-Type'] = 'application/json',
    ['Authorization'] = 'Bearer ' .. Config.APIKey,
    ['User-Agent'] = 'VanceFiveMLog-FiveM/0.1'
  }
end

local function printHTTPFailure(action, endpoint, status, body, errorData)
  local errorText = trim(errorData)
  local bodyText = trim(body)

  if status == 0 then
    if errorText == '' then
      errorText = 'no transport error detail returned by FiveM'
    end
    print(('[VanceFiveMLog] %s transport failed endpoint=%s error=%s'):format(action, endpoint or '<empty>', errorText))
    return
  end

  print(('[VanceFiveMLog] %s failed status=%s body=%s'):format(action, status, bodyText))
end

local function queueCount()
  return queueTail - queueHead + 1
end

local function compactQueue()
  if queueHead <= 1 then return end
  if queueHead > queueTail then
    queue = {}
    queueHead = 1
    queueTail = 0
    return
  end
  local newQueue = {}
  local index = 1
  for i = queueHead, queueTail do
    newQueue[index] = queue[i]
    index = index + 1
  end
  queue = newQueue
  queueTail = index - 1
  queueHead = 1
end

local function dropOldestQueuedEvent()
  if queueCount() <= 0 then return end
  queue[queueHead] = nil
  queueHead = queueHead + 1
  if queueHead > 1024 and queueHead > math.floor(queueTail / 2) then
    compactQueue()
  end
end

local function pushQueuedEvent(event)
  queueTail = queueTail + 1
  queue[queueTail] = event
end

local function popQueuedEvents(count)
  for _ = 1, count do
    if queueHead > queueTail then break end
    queue[queueHead] = nil
    queueHead = queueHead + 1
  end
  if queueHead > queueTail then
    compactQueue()
  elseif queueHead > 1024 and queueHead > math.floor(queueTail / 2) then
    compactQueue()
  end
end

local function toNumber(value)
  if type(value) == 'number' then return value end
  if type(value) == 'string' then return tonumber(value) end
  return nil
end

local function toInteger(value)
  local number = toNumber(value)
  if not number then return nil end
  if number ~= number or number == math.huge or number == -math.huge then return nil end
  return math.floor(number)
end

local function toText(value)
  if value == nil then return nil end
  if type(value) == 'string' then return value end
  if type(value) == 'number' or type(value) == 'boolean' then return tostring(value) end
  return nil
end

local function safeValue(value, depth, seen)
  local valueType = type(value)
  if value == nil or valueType == 'string' or valueType == 'boolean' then
    return value
  end
  if valueType == 'number' then
    if value ~= value or value == math.huge or value == -math.huge then return nil end
    return value
  end
  if valueType ~= 'table' then
    return tostring(value)
  end
  if depth <= 0 then
    return tostring(value)
  end
  if seen[value] then
    return tostring(value)
  end

  seen[value] = true
  local out = {}
  for key, item in pairs(value) do
    local safeKey = toText(key)
    if safeKey then
      out[safeKey] = safeValue(item, depth - 1, seen)
    end
  end
  seen[value] = nil
  return out
end

local function safeMetadata(metadata)
  if metadata == nil then return {} end
  if type(metadata) ~= 'table' then
    return { data = safeValue(metadata, 3, {}) }
  end
  local out = {}
  for key, item in pairs(metadata) do
    local safeKey = toText(key)
    if safeKey then
      out[safeKey] = safeValue(item, 3, {})
    end
  end
  return out
end

local function normalizeCoords(coords)
  if type(coords) ~= 'table' then return nil end
  local x = toNumber(coords.x or coords[1])
  local y = toNumber(coords.y or coords[2])
  local z = toNumber(coords.z or coords[3])
  if not x or not y or not z then return nil end
  return { x = x, y = y, z = z }
end

local function nowIso()
  return os.date('!%Y-%m-%dT%H:%M:%SZ')
end

local function safeCall(label, fn)
  local ok, result, extra = pcall(fn)
  if not ok then
    if Config.Debug then
      print(('[VanceFiveMLog] %s failed: %s'):format(label, tostring(result)))
    end
    return nil
  end
  return result, extra
end

local function stableEncode(value, seen)
  local valueType = type(value)
  if valueType == 'nil' then return 'nil' end
  if valueType == 'number' or valueType == 'boolean' then return tostring(value) end
  if valueType == 'string' then return ('%q'):format(value) end
  if valueType ~= 'table' then return ('%q'):format(tostring(value)) end

  seen = seen or {}
  if seen[value] then return '"<cycle>"' end
  seen[value] = true

  local keys = {}
  for key in pairs(value) do
    keys[#keys + 1] = key
  end
  table.sort(keys, function(left, right)
    return tostring(left) < tostring(right)
  end)

  local parts = {}
  for i = 1, #keys do
    local key = keys[i]
    parts[#parts + 1] = stableEncode(key, seen) .. ':' .. stableEncode(value[key], seen)
  end

  seen[value] = nil
  return '{' .. table.concat(parts, ',') .. '}'
end

local function identifiers(src)
  local out = {
    license = nil,
    discord = nil,
    steam = nil,
    identifiers = {}
  }

  for _, identifier in ipairs(GetPlayerIdentifiers(src)) do
    local prefix = identifier:match('^([^:]+):')
    if prefix and out.identifiers[prefix] == nil then
      out.identifiers[prefix] = identifier
    end
    if identifier:find('license:', 1, true) == 1 then out.license = identifier end
    if identifier:find('discord:', 1, true) == 1 then out.discord = identifier end
    if identifier:find('steam:', 1, true) == 1 then out.steam = identifier end
  end

  out.license = out.license or out.identifiers.license2
  return out
end

local function getPlayerData(src)
  local player = safeCall('qbx_core:GetPlayer', function()
    if exports.qbx_core and exports.qbx_core.GetPlayer then
      return exports.qbx_core:GetPlayer(src)
    end
    if exports['qbx_core'] and exports['qbx_core'].GetPlayer then
      return exports['qbx_core']:GetPlayer(src)
    end
    return nil
  end)

  if type(player) == 'table' then
    return player.PlayerData or player.Player or player
  end
  return nil
end

local function groupText(group)
  if type(group) ~= 'table' then return '' end
  local name = group.name or group.label or ''
  local grade = group.grade
  local gradeText = ''
  if type(grade) == 'table' then
    gradeText = tostring(grade.name or grade.level or grade.grade or '')
  elseif grade ~= nil then
    gradeText = tostring(grade)
  end
  if gradeText == '' then return tostring(name) end
  return ('%s:%s'):format(tostring(name), gradeText)
end

local function playerProfile(src)
  src = toInteger(src)
  if not src then return nil end

  local playerData = getPlayerData(src) or {}
  local charinfo = playerData.charinfo or {}
  local ids = identifiers(src)
  local first = charinfo.firstname or charinfo.firstName or ''
  local last = charinfo.lastname or charinfo.lastName or ''
  local characterName = trim(first .. ' ' .. last)
  if characterName == '' then
    characterName = playerData.name or GetPlayerName(src)
  end

  return {
    source = src,
    name = GetPlayerName(src) or playerData.name,
    character_name = characterName,
    citizenid = playerData.citizenid or playerData.citizenId,
    license = ids.license,
    discord = ids.discord,
    steam = ids.steam,
    identifiers = ids.identifiers,
    job = groupText(playerData.job),
    gang = groupText(playerData.gang)
  }
end

local function coordsFor(src)
  local ped = GetPlayerPed(src)
  if not ped or ped == 0 then return nil end
  local c = GetEntityCoords(ped)
  if not c then return nil end
  return { x = c.x, y = c.y, z = c.z }
end

local function normalizeMetadata(metadata, data)
  local out = {}

  if type(metadata) == 'table' then
    for key, item in pairs(metadata) do
      out[key] = item
    end
  elseif metadata ~= nil then
    out.data = metadata
  end

  if type(data) == 'table' then
    for key, item in pairs(data) do
      if out[key] == nil then
        out[key] = item
      end
    end
  elseif data ~= nil and out.data == nil then
    out.data = data
  end

  return out
end

local function normalize(event)
  local metadata = normalizeMetadata(event.metadata, event.data)
  local normalized = {
    event_type = toText(event.event_type or event.type or event.event),
    severity = toText(event.severity or event.level),
    source = toInteger(event.source or event.player_source),
    resource = toText(event.resource or event.plugin_resource or event.plugin),
    player_name = toText(event.player_name),
    license = toText(event.license),
    discord = toText(event.discord),
    steam = toText(event.steam),
    citizenid = toText(event.citizenid),
    message = toText(event.message),
    coords = normalizeCoords(event.coords),
    metadata = metadata,
    occurred_at = toText(event.occurred_at)
  }

  local src = normalized.source
  if src then
    local profile = playerProfile(src)
    if profile then
      normalized.player_name = normalized.player_name or profile.name
      normalized.license = normalized.license or profile.license
      normalized.discord = normalized.discord or profile.discord
      normalized.steam = normalized.steam or profile.steam
      normalized.citizenid = normalized.citizenid or profile.citizenid
      if profile.character_name and normalized.metadata.character_name == nil then
        normalized.metadata.character_name = profile.character_name
      end
      if profile.job and profile.job ~= '' and normalized.metadata.job == nil then
        normalized.metadata.job = profile.job
      end
      if profile.gang and profile.gang ~= '' and normalized.metadata.gang == nil then
        normalized.metadata.gang = profile.gang
      end
      if Config.IncludeIdentifiers and normalized.metadata.identifiers == nil then
        normalized.metadata.identifiers = profile.identifiers
      end
    else
      normalized.player_name = normalized.player_name or GetPlayerName(src)
    end
    normalized.coords = normalized.coords or coordsFor(src)
  end

  normalized.severity = normalized.severity or 'info'
  normalized.resource = normalized.resource or GetCurrentResourceName()
  if (event.plugin_resource or event.plugin) and normalized.metadata.plugin_resource == nil then
    normalized.metadata.plugin_resource = toText(event.plugin_resource or event.plugin)
  end
  normalized.occurred_at = normalized.occurred_at or nowIso()
  normalized.metadata = safeMetadata(normalized.metadata)
  return normalized
end

local function enqueue(event)
  if type(event) ~= 'table' then
    print('[VanceFiveMLog] ignored non-table log event')
    return false
  end

  if queueCount() >= Config.MaxQueue then
    dropOldestQueuedEvent()
    print('[VanceFiveMLog] queue full, dropped oldest event')
  end
  event = normalize(event)
  if not event.event_type or event.event_type == '' then
    print('[VanceFiveMLog] ignored log event without event_type')
    return false
  end

  pushQueuedEvent(event)
  return true
end

local function invokingResource()
  if type(GetInvokingResource) ~= 'function' then
    return nil
  end

  local ok, value = pcall(GetInvokingResource)
  if ok and type(value) == 'string' and value ~= '' then
    return value
  end
  return nil
end

local function pluginLogEvent(eventType, message, options)
  if type(eventType) == 'table' then
    local optionResource = type(options) == 'table' and (options.resource or options.plugin_resource or options.plugin) or nil
    eventType.resource = eventType.resource or eventType.plugin_resource or eventType.plugin or optionResource or invokingResource()
    return eventType
  end

  if type(message) == 'table' and options == nil then
    options = message
    message = options.message
  end

  if type(options) ~= 'table' then
    options = { metadata = options }
  end

  local metadata = normalizeMetadata(options.metadata, options.data)

  local resource = options.resource or options.plugin_resource or options.plugin or invokingResource() or GetCurrentResourceName()
  if metadata.plugin_resource == nil then
    metadata.plugin_resource = resource
  end

  return {
    event_type = eventType or options.event_type or options.type or options.event,
    severity = options.severity or options.level or 'info',
    source = options.source or options.player_source or options.player,
    player_name = options.player_name,
    license = options.license,
    discord = options.discord,
    steam = options.steam,
    citizenid = options.citizenid,
    resource = resource,
    message = message or options.message or eventType or options.event_type or options.type or options.event or 'plugin event',
    coords = options.coords,
    metadata = metadata,
    occurred_at = options.occurred_at
  }
end

local function readPath(value, path)
  if type(path) == 'number' then
    return value[path]
  end
  if type(path) ~= 'string' then
    return nil
  end

  local current = value
  for segment in path:gmatch('[^%.]+') do
    if type(current) ~= 'table' then return nil end
    local key = tonumber(segment) or segment
    local nextValue = current[key]
    if nextValue == nil and current[1] and type(current[1]) == 'table' then
      nextValue = current[1][key]
    end
    current = nextValue
  end
  return current
end

local function metadataValue(value)
  if type(value) ~= 'table' then
    return value
  end
  return safeValue(value, 4, {})
end

local function metadataFromMap(map, args)
  local metadata = {}
  if type(map) ~= 'table' then return metadata end

  for key, path in pairs(map) do
    metadata[key] = metadataValue(readPath(args, path))
  end
  return metadata
end

local function shouldUseBridge(bridge)
  if bridge.enabled == false then return false end
  if bridge.category and Config.Events[bridge.category] == false then return false end
  return true
end

local function registerBridge(bridge)
  if type(bridge) ~= 'table' or type(bridge.event) ~= 'string' or not shouldUseBridge(bridge) then
    return
  end

  RegisterNetEvent(bridge.event, function(...)
    local args = { ... }
    local src = nil
    if bridge.source_arg == 'source' then
      src = source
    elseif bridge.source_arg then
      src = readPath(args, bridge.source_arg)
    else
      src = source
    end

    enqueue({
      event_type = bridge.event_type or bridge.event,
      severity = bridge.severity or 'info',
      source = src,
      resource = bridge.resource,
      message = bridge.message or bridge.event,
      metadata = metadataFromMap(bridge.metadata, args)
    })
  end)
end

local function inventoryEnabled()
  return Config.Events.items ~= false and Config.Inventory and Config.Inventory.Enabled ~= false
end

local function inventoryProvider()
  if Config.Inventory and Config.Inventory.Provider then
    return Config.Inventory.Provider
  end
  return 'ox_inventory'
end

local function inventoryConfig(name, fallback)
  if Config.Inventory and Config.Inventory[name] ~= nil then
    return Config.Inventory[name]
  end
  return fallback
end

local function inventoryItemAllowed(itemName)
  local ignored = Config.Inventory and Config.Inventory.IgnoredItems
  return not ignored or ignored[itemName] ~= true
end

local function inventoryMetadataKey(metadata)
  return stableEncode(metadata or {})
end

local function getInventoryItems(src)
  return safeCall('ox_inventory:GetInventoryItems', function()
    return exports.ox_inventory:GetInventoryItems(src)
  end) or {}
end

local function buildInventorySnapshot(src)
  local snapshot = {}
  local items = getInventoryItems(src)
  if type(items) ~= 'table' then return snapshot end

  for _, item in pairs(items) do
    if type(item) == 'table' and item.name and inventoryItemAllowed(item.name) then
      local count = toNumber(item.count) or 0
      if count > 0 then
        local metadata = item.metadata or {}
        local key = tostring(item.name) .. '|' .. inventoryMetadataKey(metadata)
        local entry = snapshot[key]
        if not entry then
          entry = {
            name = item.name,
            label = item.label or item.name,
            metadata = metadata,
            count = 0
          }
          snapshot[key] = entry
        end
        entry.count = entry.count + count
      end
    end
  end

  return snapshot
end

local function diffInventory(before, after)
  local changes = {}
  before = before or {}
  after = after or {}

  for key, afterEntry in pairs(after) do
    local beforeEntry = before[key]
    local beforeCount = beforeEntry and beforeEntry.count or 0
    local delta = afterEntry.count - beforeCount
    if delta ~= 0 then
      changes[#changes + 1] = {
        key = key,
        name = afterEntry.name,
        label = afterEntry.label,
        metadata = afterEntry.metadata,
        before = beforeCount,
        after = afterEntry.count,
        delta = delta
      }
    end
  end

  for key, beforeEntry in pairs(before) do
    if after[key] == nil then
      changes[#changes + 1] = {
        key = key,
        name = beforeEntry.name,
        label = beforeEntry.label,
        metadata = beforeEntry.metadata,
        before = beforeEntry.count,
        after = 0,
        delta = -beforeEntry.count
      }
    end
  end

  table.sort(changes, function(left, right)
    return left.key < right.key
  end)
  return changes
end

local function describeInventoryContext(context)
  if type(context) ~= 'table' then return nil end
  local parts = {}
  if context.event then parts[#parts + 1] = ('event=%s'):format(context.event) end
  if context.action then parts[#parts + 1] = ('action=%s'):format(context.action) end
  if context.fromType then parts[#parts + 1] = ('from=%s'):format(context.fromType) end
  if context.toType then parts[#parts + 1] = ('to=%s'):format(context.toType) end
  if context.itemName then parts[#parts + 1] = ('item=%s'):format(context.itemName) end
  if context.shopType then parts[#parts + 1] = ('shop=%s'):format(context.shopType) end
  if context.benchId then parts[#parts + 1] = ('bench=%s'):format(context.benchId) end
  if context.consume then parts[#parts + 1] = ('consume=%s'):format(context.consume) end
  if #parts == 0 then return nil end
  return table.concat(parts, ' ')
end

local function summarizeInventoryChanges(changes)
  local maxItems = inventoryConfig('EventMaxChanges', 8)
  local parts = {}
  for i = 1, math.min(#changes, maxItems) do
    local change = changes[i]
    local direction = change.delta > 0 and '+' or ''
    parts[#parts + 1] = ('%s %s%s'):format(change.label or change.name, direction, change.delta)
  end
  if #changes > maxItems then
    parts[#parts + 1] = ('+%s more'):format(#changes - maxItems)
  end
  return table.concat(parts, ', ')
end

local function inventorySnapshot(src)
  src = toInteger(src)
  if not src or GetPlayerName(src) == nil then return end
  if not inventoryEnabled() or inventoryProvider() ~= 'ox_inventory' then return end
  inventorySnapshots[src] = buildInventorySnapshot(src)
  if Config.Debug then
    local groups = 0
    for _ in pairs(inventorySnapshots[src]) do groups = groups + 1 end
    print(('[VanceFiveMLog] inventory snapshot initialized source=%s groups=%s'):format(src, groups))
  end
end

local function inventoryForget(src)
  src = toInteger(src)
  if not src then return end
  inventorySnapshots[src] = nil
  inventoryPending[src] = nil
  inventoryContext[src] = nil
end

local function inventoryCheck(src, context)
  src = toInteger(src)
  if not src or GetPlayerName(src) == nil then
    inventoryForget(src)
    return
  end
  if not inventoryEnabled() or inventoryProvider() ~= 'ox_inventory' then
    inventoryForget(src)
    return
  end

  local before = inventorySnapshots[src]
  local after = buildInventorySnapshot(src)
  inventorySnapshots[src] = after
  if not before then return end

  local changes = diffInventory(before, after)
  if #changes == 0 then return end

  local severity = 'info'
  for _, change in ipairs(changes) do
    if change.delta < 0 then
      severity = 'warning'
      break
    end
  end

  local safeChanges = {}
  for _, change in ipairs(changes) do
    safeChanges[#safeChanges + 1] = {
      name = change.name,
      label = change.label,
      metadata = metadataValue(change.metadata),
      before = change.before,
      after = change.after,
      delta = change.delta
    }
  end

  enqueue({
    event_type = 'inventory_diff',
    severity = severity,
    source = src,
    resource = 'ox_inventory',
    message = 'inventory changed: ' .. summarizeInventoryChanges(changes),
    metadata = {
      category = 'inventory',
      action = 'diff',
      change_count = #changes,
      context = metadataValue(context or {}),
      context_text = describeInventoryContext(context),
      changes = safeChanges
    }
  })
end

local function inventoryScheduleCheck(src, context, delay)
  src = toInteger(src)
  if not src or GetPlayerName(src) == nil then return end
  if not inventoryEnabled() or inventoryProvider() ~= 'ox_inventory' then return end

  inventoryContext[src] = context or inventoryContext[src]
  if inventoryPending[src] then return end
  inventoryPending[src] = true

  SetTimeout(delay or inventoryConfig('HookDiffDelayMs', 350), function()
    inventoryPending[src] = nil
    local latestContext = inventoryContext[src]
    inventoryContext[src] = nil
    inventoryCheck(src, latestContext)
  end)
end

local function numericInventory(value)
  if type(value) == 'number' then return value end
  if type(value) == 'string' then return tonumber(value) end
  if type(value) == 'table' then return tonumber(value.id or value.owner or value.inventoryId) end
  return nil
end

local function inventoryPayloadSources(payload)
  local sources = {}
  if type(payload) ~= 'table' then return sources end

  local direct = tonumber(payload.source)
  if direct then sources[direct] = true end

  local fromSource = numericInventory(payload.fromInventory)
  local toSource = numericInventory(payload.toInventory)
  local inventoryId = numericInventory(payload.inventoryId)
  if payload.fromType == 'player' and fromSource then sources[fromSource] = true end
  if payload.toType == 'player' and toSource then sources[toSource] = true end
  if inventoryId then sources[inventoryId] = true end

  return sources
end

local function scheduleInventoryPayloadDiff(eventName, payload)
  payload = payload or {}
  local context = {
    event = eventName,
    action = payload.action,
    fromType = payload.fromType,
    toType = payload.toType,
    itemName = payload.itemName or (payload.item and payload.item.name) or (payload.recipe and payload.recipe.name),
    shopType = payload.shopType,
    benchId = payload.benchId,
    consume = payload.consume
  }

  for src in pairs(inventoryPayloadSources(payload)) do
    inventoryScheduleCheck(src, context)
  end
end

local function registerInventoryHook(eventName)
  local hookID = safeCall(('ox_inventory:registerHook:%s'):format(eventName), function()
    return exports.ox_inventory:registerHook(eventName, function(payload)
      scheduleInventoryPayloadDiff(eventName, payload or {})
    end)
  end)
  if hookID then
    inventoryHooks[#inventoryHooks + 1] = hookID
    if Config.Debug then
      print(('[VanceFiveMLog] registered ox_inventory hook %s as %s'):format(eventName, hookID))
    end
  end
end

local function currentSources()
  local sources = {}
  for _, player in ipairs(GetPlayers()) do
    local src = tonumber(player)
    if src then sources[#sources + 1] = src end
  end
  return sources
end

local function startInventoryAuditing()
  if inventoryStarted or not inventoryEnabled() then return end
  if inventoryProvider() ~= 'ox_inventory' then
    print(('[VanceFiveMLog] inventory logging disabled: unsupported provider %s'):format(tostring(inventoryProvider())))
    return
  end
  if GetResourceState('ox_inventory') ~= 'started' then
    print('[VanceFiveMLog] inventory logging disabled: ox_inventory is not started')
    return
  end

  inventoryStarted = true
  for _, eventName in ipairs({ 'swapItems', 'createItem', 'buyItem', 'craftItem', 'usingItem' }) do
    registerInventoryHook(eventName)
  end

  SetTimeout(inventoryConfig('InitialSnapshotDelayMs', 2500), function()
    for _, src in ipairs(currentSources()) do
      inventorySnapshot(src)
    end
  end)

  CreateThread(function()
    while inventoryStarted do
      Wait(inventoryConfig('ScanIntervalMs', 2000))
      for _, src in ipairs(currentSources()) do
        inventoryCheck(src, { event = 'scanner' })
      end
    end
  end)
end

local function flush()
  if flushing or queueCount() == 0 then return end
  flushing = true

  local batch = {}
  local count = math.min(queueCount(), Config.BatchSize)
  for i = 1, count do
    batch[i] = queue[queueHead + i - 1]
  end

  PerformHttpRequest(Config.Endpoint, function(status, body, _headers, errorData)
    if status >= 200 and status < 300 then
      popQueuedEvents(count)
      if Config.Debug then
        print(('[VanceFiveMLog] flushed %s events'):format(count))
      end
    elseif status == 400 then
      popQueuedEvents(count)
      print(('[VanceFiveMLog] dropped %s rejected events status=%s body=%s'):format(count, status, body or ''))
    else
      printHTTPFailure('flush', Config.Endpoint, status, body, errorData)
    end
    flushing = false
  end, 'POST', json.encode({ events = batch }), requestHeaders())
end

local function heartbeat()
  if heartbeatInFlight then return end
  if not Config.HeartbeatEndpoint or Config.HeartbeatEndpoint == '' then return end
  heartbeatInFlight = true

  PerformHttpRequest(Config.HeartbeatEndpoint, function(status, body, _headers, errorData)
    if status >= 200 and status < 300 then
      if Config.Debug then
        print('[VanceFiveMLog] heartbeat ok')
      end
    else
      printHTTPFailure('heartbeat', Config.HeartbeatEndpoint, status, body, errorData)
    end
    heartbeatInFlight = false
  end, 'POST', json.encode({
    resource = GetCurrentResourceName(),
    queued = queueCount(),
    players = #GetPlayers(),
    uptime = GetGameTimer()
  }), requestHeaders())
end

exports('LogEvent', function(event)
  if type(event) == 'table' then
    event.resource = event.resource or event.plugin_resource or event.plugin or invokingResource()
  end
  return enqueue(event)
end)

exports('Log', function(eventType, message, options)
  return enqueue(pluginLogEvent(eventType, message, options))
end)

exports('Flush', function()
  flush()
end)

AddEventHandler('vancefivemlog:server:Log', function(eventType, message, options)
  local event = pluginLogEvent(eventType, message, options)
  event.resource = event.resource or invokingResource()
  enqueue(event)
end)

AddEventHandler('vancefivemlog:server:Flush', function()
  flush()
end)

CreateThread(function()
  printConfigStatus()
  Wait(2500)
  heartbeat()
  startInventoryAuditing()
end)

CreateThread(function()
  while true do
    Wait(Config.FlushIntervalMs)
    flush()
  end
end)

CreateThread(function()
  while true do
    Wait(Config.HeartbeatIntervalMs)
    heartbeat()
  end
end)

CreateThread(function()
  if type(Config.EventBridge) ~= 'table' then return end
  for _, bridge in ipairs(Config.EventBridge) do
    registerBridge(bridge)
  end
end)

AddEventHandler('playerConnecting', function(name)
  if not Config.Events.playerConnecting then return end
  local src = source
  enqueue({
    event_type = 'player_connecting',
    severity = 'info',
    source = src,
    player_name = name,
    message = ('%s is connecting'):format(name)
  })
end)

AddEventHandler('playerDropped', function(reason)
  local src = source
  if Config.Events.playerDropped then
    enqueue({
      event_type = 'player_dropped',
      severity = 'warning',
      source = src,
      message = ('%s disconnected: %s'):format(GetPlayerName(src) or 'unknown', reason or 'unknown'),
      metadata = { reason = reason }
    })
  end
  inventoryForget(src)
end)

AddEventHandler('chatMessage', function(src, name, message)
  if not Config.Events.chat then return end
  enqueue({
    event_type = 'chat_message',
    severity = 'info',
    source = src,
    player_name = name,
    message = message,
    metadata = { channel = 'global' }
  })
end)

RegisterNetEvent('QBCore:Server:OnPlayerLoaded', function()
  local src = toInteger(source)
  if not src then return end
  SetTimeout(1000, function()
    inventorySnapshot(src)
  end)
end)

AddEventHandler('QBCore:Server:OnPlayerUnload', function(src)
  src = toInteger(src) or toInteger(source)
  inventoryForget(src)
end)

AddEventHandler('QBCore:Server:OnMoneyChange', function(src, moneyType, amount, operation, reason)
  if Config.Events.money == false then return end
  src = toInteger(src)
  if not src then return end

  local moneyConfig = Config.MoneyTypes or { cash = true, bank = true }
  if moneyConfig[moneyType] ~= true then return end

  local balance = safeCall('qbx_core:GetMoney', function()
    return exports.qbx_core:GetMoney(src, moneyType)
  end)

  local severity = 'warning'
  if operation == 'add' then severity = 'info' end
  if operation == 'set' then severity = 'warning' end
  if operation == 'remove' then severity = 'warning' end

  enqueue({
    event_type = 'money_change',
    severity = severity,
    source = src,
    resource = 'qbx_core',
    message = ('%s %s %s: %s'):format(tostring(moneyType or 'money'), tostring(operation or 'changed'), tostring(amount or 0), tostring(reason or 'unknown')),
    metadata = {
      category = 'money',
      money_type = moneyType,
      amount = toNumber(amount) or amount,
      operation = operation,
      reason = reason,
      balance = balance
    }
  })
end)

RegisterNetEvent('baseevents:onPlayerDied', function(killerType, deathCoords)
  if not Config.Events.death then return end
  local src = source
  enqueue({
    event_type = 'player_death',
    severity = 'warning',
    source = src,
    message = ('%s died'):format(GetPlayerName(src) or 'unknown'),
    coords = deathCoords,
    metadata = { killer_type = killerType }
  })
end)

RegisterNetEvent('baseevents:onPlayerKilled', function(killerId, data)
  if not Config.Events.death then return end
  local src = source
  enqueue({
    event_type = 'player_killed',
    severity = 'error',
    source = src,
    message = ('%s killed by %s'):format(GetPlayerName(src) or 'unknown', GetPlayerName(killerId) or killerId),
    metadata = { killer = killerId, data = data }
  })
end)

RegisterNetEvent('VanceFiveMLog:server:LogEvent', function(event)
  enqueue(event)
end)

AddEventHandler('onResourceStop', function(resource)
  if resource == GetCurrentResourceName() and inventoryStarted and inventoryProvider() == 'ox_inventory' and GetResourceState('ox_inventory') == 'started' then
    for _, hookID in ipairs(inventoryHooks) do
      safeCall('ox_inventory:removeHooks', function()
        exports.ox_inventory:removeHooks(hookID)
      end)
    end
    inventoryStarted = false
  end

  if not Config.Events.resource then return end
  enqueue({
    event_type = 'resource_stop',
    severity = 'warning',
    resource = resource,
    message = ('resource stopped: %s'):format(resource)
  })
  flush()
end)
