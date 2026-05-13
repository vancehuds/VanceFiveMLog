# VanceFiveMLog 接入指南

[English Version](INTEGRATION.en.md)

本文档面向需要把其他 FiveM 插件、Qbox/QBCore 资源或外部服务接入 VanceFiveMLog 的开发者。

## 1. 接入方式选择

推荐优先级：

1. **FiveM 服务端资源内接入**：使用 `exports.vancefivemlog:Log(...)`。
2. **事件桥接配置**：在 `fivem-resource/config.lua` 的 `Config.EventBridge` 中配置事件桥接。
3. **外部系统接入**：直接请求后端 `POST /api/v1/events`。

服务端资源内接入是推荐方式，因为 `vancefivemlog` 会根据 `source` 自动补全玩家名称、license、discord、steam、citizenid 和坐标。

## 2. 前置准备

先确保日志资源已经安装并启动：

```cfg
ensure vancefivemlog
ensure your_resource
```

FiveM 的 export 名称来自资源名。请把本项目的 `fivem-resource` 文件夹复制/重命名为 `vancefivemlog`，例如 `resources/[logging]/vancefivemlog`。如果实际目录仍叫 `fivem-resource` 或其他名字，`exports.vancefivemlog:Log(...)` 可能找不到导出。

如果你的资源会调用导出，建议在你的 `fxmanifest.lua` 中声明依赖：

```lua
dependency 'vancefivemlog'
```

**推荐**：在 `server.cfg` 使用仅服务端可见的 convar 配置后台地址和 API Key，避免把密钥写进资源目录：

```cfg
set vfl_endpoint "https://你的后台域名/api/v1/events"
set vfl_heartbeat_endpoint "https://你的后台域名/api/v1/heartbeat"
set vfl_api_key "后台系统设置中创建服务器时生成的APIKey"
```

也可以直接修改 `vancefivemlog/config.lua`：

```lua
Config.Endpoint = 'https://你的后台域名/api/v1/events'
Config.HeartbeatEndpoint = 'https://你的后台域名/api/v1/heartbeat'
Config.APIKey = '后台系统设置中创建服务器时生成的APIKey'
```

## 3. 推荐接入：FiveM Export

基础用法：

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

函数签名：

```lua
exports.vancefivemlog:Log(eventType, message, options)
```

### 安全包装模式

推荐在改造第三方插件时使用一个本地 helper，这样日志资源未启动或导出调用失败时，原插件不会中断，同时会在控制台打印失败原因：

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

### 参数说明

- `eventType`：事件类型，建议使用稳定的英文标识，例如 `inventory_remove`、`money_change`、`admin_ban`。
- `message`：展示在日志列表中的简短描述。
- `options.severity`：严重等级，支持 `info`、`success`、`warning`、`error`。
- `options.source`：FiveM 玩家 server id。传入后会自动补玩家信息和坐标。
- `options.metadata`：业务字段，会写入后端 `metadata`。
- `options.resource`：手动覆盖资源名；不传时会自动使用调用方资源名。
- `options.coords`：手动传坐标，格式为 `{ x = 1.0, y = 2.0, z = 3.0 }`。
- `options.occurred_at`：自定义事件发生时间，使用 ISO/RFC3339 字符串。

`message` 也可以省略，把配置表作为第二个参数：

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

也可以直接传完整事件表：

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

## 4. 兼容字段

为了降低第三方插件适配成本，资源侧和后端都会兼容以下字段别名：

| 推荐字段     | 兼容字段                    | 说明                 |
| ------------ | --------------------------- | -------------------- |
| `event_type` | `type`, `event`             | 事件类型             |
| `severity`   | `level`                     | 严重等级             |
| `source`     | `player_source`, `player`   | FiveM 玩家 server id |
| `resource`   | `plugin`, `plugin_resource` | 插件或资源名         |
| `metadata`   | `data`                      | 业务附加数据         |

如果事件类型包含空格，后端会归一化为下划线，例如 `door forced` 会变成 `door_forced`。

## 5. 常见插件示例

### 背包变更

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

### 金钱变更

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

### 管理员操作

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

### 立即刷新

默认资源会按 `Config.FlushIntervalMs` 批量上报。如果某些关键事件需要尽快写入，可以调用：

```lua
exports.vancefivemlog:Log('admin_ban', 'player banned', {
  severity = 'error',
  source = target,
  metadata = { reason = reason }
})

exports.vancefivemlog:Flush()
```

不要在高频事件中每次都调用 `Flush()`，否则会增加 HTTP 请求量。

## 6. 事件桥接接入

如果你不想改第三方插件源码，可以在 `fivem-resource/config.lua` 中添加 `Config.EventBridge`。

示例：

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

字段说明：

- `event`：要监听的 FiveM server event 名称。
- `event_type`：写入日志系统的事件类型。
- `category`：对应 `Config.Events` 中的开关。
- `severity`：严重等级。
- `source_arg`：玩家来源，可以是 `source`、参数索引或表字段路径。
- `metadata`：从事件参数中提取的业务字段。

## 7. 直接 HTTP 接入

外部服务可以直接请求后端：

```http
POST /api/v1/events
Authorization: Bearer <server_api_key>
Content-Type: application/json
```

单事件：

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

批量事件：

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

也兼容原始包装格式：

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

后端单次最多接收 500 条事件，请对高频日志做批量或节流。

## 8. 字段规范建议

### 事件命名

- 使用小写英文和下划线，例如 `inventory_remove`。
- 同一类事件保持稳定名称，不要把动态值写进 `event_type`。
- 动态值放进 `metadata`，例如物品名、金额、车牌、原因。

### 严重等级

- `info`：普通行为，例如进入区域、普通物品变化。
- `success`：成功完成的关键流程，例如交易完成。
- `warning`：需要关注的行为，例如大额金钱变化、删除物品、管理员警告。
- `error`：高风险或失败事件，例如封禁、反作弊命中、异常失败。

### 隐私和性能

- 不要把密码、token、银行卡号等敏感信息写入 `metadata`。
- 高频事件应聚合后上报，避免每帧、每 tick 写日志。
- `message` 保持简短，详细上下文放到 `metadata`。

## 9. 排错

- **FiveM 控制台出现 `unauthorized`**：检查 `Config.APIKey` 是否是后台当前服务器的 API Key，且没有多余空格。
- **FiveM 控制台出现 `status=0`**：请求没有拿到 HTTP 响应，通常是 FXServer 所在机器无法访问域名、DNS 解析失败、TLS/证书失败、防火墙拦截，或实际加载的 endpoint 不是公网地址。先看资源启动时打印的 endpoint 和 `transport failed ... error=...`。
- **控制台提示找不到 `exports.vancefivemlog`**：确认日志资源目录名是 `vancefivemlog`，`server.cfg` 里先 `ensure vancefivemlog` 再启动调用它的插件；改造后的插件只在服务端 Lua 调用 export，不要放在 client 脚本里。
- **日志没有显示资源名**：确认通过 `exports.vancefivemlog:Log(...)` 调用，或手动传 `resource`/`plugin_resource`。
- **玩家信息为空**：确认传入的是服务端玩家 `source`，不是 license、identifier 或 client id。
- **后端返回 `invalid log event`**：确认至少传了 `event_type`、`type` 或 `event`。
- **坐标为空**：玩家 ped 不存在、玩家刚断开或事件不是玩家事件时，坐标可能为空。
- **高频事件丢失**：检查 `Config.MaxQueue`、`Config.BatchSize`、后台网络连通性和 FiveM 控制台 HTTP 状态码。
