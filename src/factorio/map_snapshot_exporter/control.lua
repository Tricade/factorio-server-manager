local export_root = "fsm-map-exporter"
local batch_size = 64
local fallback_chart_requested = false
local chunk_size = 32

local transient_entity_types = {
    ["character"] = true,
    ["combat-robot"] = true,
    ["construction-robot"] = true,
    ["corpse"] = true,
    ["highlight-box"] = true,
    ["item-entity"] = true,
    ["item-request-proxy"] = true,
    ["logistic-robot"] = true,
    ["particle-source"] = true,
    ["projectile"] = true,
    ["smoke-with-trigger"] = true,
    ["speech-bubble"] = true
}

local function ordered_surfaces()
    local surfaces = {}
    for _, surface in pairs(game.surfaces) do
        surfaces[#surfaces + 1] = surface
    end
    table.sort(surfaces, function(left, right)
        return left.index < right.index
    end)
    return surfaces
end

local function player_force()
    if game.forces.player then
        return game.forces.player
    end
    for _, force in pairs(game.forces) do
        if force.name ~= "enemy" and force.name ~= "neutral" then
            return force
        end
    end
    return nil
end

local function flush_lines(filename, lines)
    if #lines == 0 then
        return
    end
    helpers.write_file(filename, table.concat(lines, "\n") .. "\n", true)
    for index = #lines, 1, -1 do
        lines[index] = nil
    end
end

local function surface_identity(surface)
    local platform = surface.platform
    if platform and platform.valid then
        local name = platform.name
        if name and name ~= "" then
            return name, "platform"
        end
        return surface.name, "platform"
    end
    if surface.planet then
        return surface.name, "planet"
    end
    return surface.name, "surface"
end

local function surface_content_bounds(surface, force)
    local min_x = nil
    local min_y = nil
    local max_x = nil
    local max_y = nil

    for _, entity in pairs(surface.find_entities_filtered({force = force})) do
        if entity.valid and not transient_entity_types[entity.type] then
            local box = entity.bounding_box
            local left_top = box and box.left_top or entity.position
            local right_bottom = box and box.right_bottom or entity.position
            min_x = min_x and math.min(min_x, left_top.x) or left_top.x
            min_y = min_y and math.min(min_y, left_top.y) or left_top.y
            max_x = max_x and math.max(max_x, right_bottom.x) or right_bottom.x
            max_y = max_y and math.max(max_y, right_bottom.y) or right_bottom.y
        end
    end

    return min_x, min_y, max_x, max_y
end

local function fitted_tile_bounds(surface, force, min_x, min_y, max_x, max_y)
    local content_min_x, content_min_y, content_max_x, content_max_y = surface_content_bounds(surface, force)
    if not content_min_x then
        return min_x * chunk_size, min_y * chunk_size, (max_x + 1) * chunk_size - 1, (max_y + 1) * chunk_size - 1
    end

    local padding = surface.platform and 8 or 64
    local chart_min_x = min_x * chunk_size
    local chart_min_y = min_y * chunk_size
    local chart_max_x = (max_x + 1) * chunk_size - 1
    local chart_max_y = (max_y + 1) * chunk_size - 1
    local view_min_x = math.max(chart_min_x, math.floor(content_min_x) - padding)
    local view_min_y = math.max(chart_min_y, math.floor(content_min_y) - padding)
    local view_max_x = math.min(chart_max_x, math.ceil(content_max_x) + padding)
    local view_max_y = math.min(chart_max_y, math.ceil(content_max_y) + padding)
    if view_min_x > view_max_x or view_min_y > view_max_y then
        return chart_min_x, chart_min_y, chart_max_x, chart_max_y
    end
    return view_min_x, view_min_y, view_max_x, view_max_y
end

local function export_map()
    helpers.remove_path(export_root)

    local force = player_force()
    local manifest = {
        schema_version = 1,
        game_tick = game.tick,
        game_version = helpers.game_version,
        force = force and force.name or "",
        surfaces = {}
    }

    if force then
        for _, surface in ipairs(ordered_surfaces()) do
            local filename = export_root .. "/surface-" .. surface.index .. ".jsonl"
            local lines = {}
            local chunk_count = 0
            local min_x = nil
            local min_y = nil
            local max_x = nil
            local max_y = nil

            for chunk in surface.get_chunks() do
                local pixels = force.get_chunk_chart(surface, chunk)
                if pixels then
                    local encoded = helpers.encode_string(pixels)
                    if encoded then
                        chunk_count = chunk_count + 1
                        min_x = min_x and math.min(min_x, chunk.x) or chunk.x
                        min_y = min_y and math.min(min_y, chunk.y) or chunk.y
                        max_x = max_x and math.max(max_x, chunk.x) or chunk.x
                        max_y = max_y and math.max(max_y, chunk.y) or chunk.y
                        lines[#lines + 1] = helpers.table_to_json({x = chunk.x, y = chunk.y, data = encoded})
                        if #lines >= batch_size then
                            flush_lines(filename, lines)
                        end
                    end
                end
            end
            flush_lines(filename, lines)

            if chunk_count > 0 then
                local display_name, kind = surface_identity(surface)
                local view_min_x, view_min_y, view_max_x, view_max_y = fitted_tile_bounds(surface, force, min_x, min_y, max_x, max_y)
                manifest.surfaces[#manifest.surfaces + 1] = {
                    index = surface.index,
                    name = display_name,
                    surface_name = surface.name,
                    kind = kind,
                    chunk_count = chunk_count,
                    min_x = min_x,
                    min_y = min_y,
                    max_x = max_x,
                    max_y = max_y,
                    view_min_tile_x = view_min_x,
                    view_min_tile_y = view_min_y,
                    view_max_tile_x = view_max_x,
                    view_max_tile_y = view_max_y,
                    file = "surface-" .. surface.index .. ".jsonl"
                }
            end
        end
    end

    helpers.write_file(export_root .. "/manifest.json", helpers.table_to_json(manifest), false)
    helpers.write_file(export_root .. "/complete", "ok\n", false)
end

local function has_chart_data(force)
    if not force then
        return false
    end
    for _, surface in ipairs(ordered_surfaces()) do
        for chunk in surface.get_chunks() do
            if force.get_chunk_chart(surface, chunk) then
                return true
            end
        end
    end
    return false
end

script.on_event(defines.events.on_tick, function()
    local force = player_force()
    if not force then
        export_map()
        script.on_event(defines.events.on_tick, nil)
        return
    end
    if not fallback_chart_requested and not has_chart_data(force) then
        for _, surface in ipairs(ordered_surfaces()) do
            force.chart_all(surface)
        end
        fallback_chart_requested = true
        return
    end
    export_map()
    script.on_event(defines.events.on_tick, nil)
end)
