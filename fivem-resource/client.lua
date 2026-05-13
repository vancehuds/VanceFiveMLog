RegisterNetEvent('VanceFiveMLog:client:LogCoords', function(eventType, message, severity)
  local ped = PlayerPedId()
  local coords = GetEntityCoords(ped)
  TriggerServerEvent('VanceFiveMLog:server:LogEvent', {
    event_type = eventType or 'client_coords',
    severity = severity or 'info',
    source = GetPlayerServerId(PlayerId()),
    message = message or 'client coordinate snapshot',
    coords = { x = coords.x, y = coords.y, z = coords.z },
    metadata = {}
  })
end)
