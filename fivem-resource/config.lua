Config = {}

Config.Endpoint = 'http://127.0.0.1:8080/api/v1/events'
Config.HeartbeatEndpoint = 'http://127.0.0.1:8080/api/v1/heartbeat'
Config.APIKey = 'replace-with-generated-api-key'

Config.FlushIntervalMs = 5000
Config.HeartbeatIntervalMs = 30000
Config.BatchSize = 250
Config.MaxQueue = 5000
Config.Debug = false
Config.IncludeIdentifiers = true

Config.Events = {
  playerConnecting = true,
  playerDropped = true,
  chat = true,
  death = true,
  money = true,
  items = true,
  jobs = true,
  vehicles = true,
  admin = true,
  resource = true
}

Config.MoneyTypes = {
  cash = true,
  bank = true
}

Config.Inventory = {
  Enabled = true,
  Provider = 'ox_inventory',
  ScanIntervalMs = 2000,
  HookDiffDelayMs = 350,
  InitialSnapshotDelayMs = 2500,
  EventMaxChanges = 8,
  IgnoredItems = {}
}

-- Resource-specific hooks are intentionally configurable because inventory,
-- vehicle, and admin resources often rename their events between servers.
Config.EventBridge = {
  {
    event = 'ox_inventory:removeItem',
    enabled = true,
    event_type = 'inventory_remove',
    category = 'items',
    severity = 'warning',
    source_arg = 1,
    message = 'inventory item removed',
    metadata = {
      item = 2,
      count = 3,
      reason = 4
    }
  },
  {
    event = 'ox_inventory:addItem',
    enabled = true,
    event_type = 'inventory_add',
    category = 'items',
    severity = 'info',
    source_arg = 1,
    message = 'inventory item added',
    metadata = {
      item = 2,
      count = 3,
      reason = 4
    }
  },
  {
    event = 'QBCore:Server:OnMoneyChange',
    enabled = false, -- built-in money audit handles this event; enable only for custom mappings.
    event_type = 'money_change',
    category = 'money',
    severity = 'warning',
    source_arg = 1,
    message = 'player money changed',
    metadata = {
      money_type = 2,
      amount = 3,
      operation = 4,
      reason = 5
    }
  },
  {
    event = 'qbx_core:server:onGroupUpdate',
    enabled = true,
    event_type = 'job_update',
    category = 'jobs',
    severity = 'info',
    source_arg = 1,
    message = 'player group updated',
    metadata = {
      group = 2,
      grade = 3
    }
  },
  {
    event = 'qb-vehiclekeys:server:AcquireVehicleKeys',
    enabled = true,
    event_type = 'vehicle_keys_acquired',
    category = 'vehicles',
    severity = 'info',
    source_arg = 1,
    message = 'vehicle keys acquired',
    metadata = {
      plate = 2
    }
  },
  {
    event = 'txAdmin:events:playerWarned',
    enabled = true,
    event_type = 'admin_warn',
    category = 'admin',
    severity = 'warning',
    source_arg = 'target',
    message = 'txAdmin warning issued',
    metadata = {
      action_id = 'actionId',
      author = 'author',
      reason = 'reason'
    }
  },
  {
    event = 'txAdmin:events:playerBanned',
    enabled = true,
    event_type = 'admin_ban',
    category = 'admin',
    severity = 'error',
    source_arg = 'target',
    message = 'txAdmin ban issued',
    metadata = {
      action_id = 'actionId',
      author = 'author',
      reason = 'reason',
      expiration = 'expiration'
    }
  }
}
