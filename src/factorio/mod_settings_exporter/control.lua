local exporter_name = "fsm-mod-settings-exporter"
local output_root = exporter_name .. "/"

local function write_output(path, contents)
    if helpers and helpers.write_file then
        helpers.write_file(output_root .. path, contents, false)
    else
        game.write_file(output_root .. path, contents, false)
    end
end

local function to_json(value)
    if helpers and helpers.table_to_json then
        return helpers.table_to_json(value)
    end
    return game.table_to_json(value)
end

local function mod_setting_prototypes()
    if prototypes and prototypes.mod_setting then
        return prototypes.mod_setting
    end
    return game.mod_setting_prototypes
end

local function normalized_value(prototype_type, value)
    if prototype_type == "color-setting" and value then
        return {
            r = value.r or 0,
            g = value.g or 0,
            b = value.b or 0,
            a = value.a == nil and 1 or value.a
        }
    end
    return value
end

local function export_startup_settings()
    local source = mod_setting_prototypes()
    local names = {}
    for name, prototype in pairs(source) do
        if prototype.setting_type == "startup" and not prototype.hidden then
            names[#names + 1] = name
        end
    end
    table.sort(names)

    local result = {}
    for index, name in ipairs(names) do
        local prototype = source[name]
        local current = settings.startup[name]
        local current_value = prototype.default_value
        if current then
            current_value = current.value
        end
        local allowed = nil
        if prototype.allowed_values then
            allowed = {}
            for allowed_index, allowed_value in pairs(prototype.allowed_values) do
                allowed[allowed_index] = allowed_value
            end
        end
        result[#result + 1] = {
            name = name,
            mod = prototype.mod or "unknown",
            type = prototype.type,
            order = prototype.order or "",
            current_value = normalized_value(prototype.type, current_value),
            default_value = normalized_value(prototype.type, prototype.default_value),
            minimum_value = prototype.minimum_value,
            maximum_value = prototype.maximum_value,
            allowed_values = allowed,
            allow_blank = prototype.allow_blank,
            auto_trim = prototype.auto_trim,
            locale_index = index
        }
        write_output("locale/name-" .. index .. ".txt", prototype.localised_name or name)
        write_output("locale/description-" .. index .. ".txt", prototype.localised_description or "")
    end

    write_output("manifest.json", to_json({schema_version = 1, settings = result}))
    write_output("complete", "ok")
end

script.on_init(export_startup_settings)
script.on_event(defines.events.on_tick, function()
    export_startup_settings()
    script.on_event(defines.events.on_tick, nil)
end)
