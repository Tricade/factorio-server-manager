data:extend({
    {
        type = "bool-setting",
        name = "fsm-fixture-bool",
        setting_type = "startup",
        default_value = true,
        order = "a"
    },
    {
        type = "int-setting",
        name = "fsm-fixture-int",
        setting_type = "startup",
        default_value = 5,
        minimum_value = -10,
        maximum_value = 10,
        order = "b"
    },
    {
        type = "double-setting",
        name = "fsm-fixture-double",
        setting_type = "startup",
        default_value = 1.25,
        minimum_value = 0.5,
        maximum_value = 4,
        order = "c"
    },
    {
        type = "string-setting",
        name = "fsm-fixture-choice",
        setting_type = "startup",
        default_value = "alpha",
        allowed_values = {"alpha", "beta"},
        order = "d"
    },
    {
        type = "color-setting",
        name = "fsm-fixture-color",
        setting_type = "startup",
        default_value = {r = 0.25, g = 0.5, b = 0.75, a = 1},
        order = "e"
    },
    {
        type = "string-setting",
        name = "fsm-fixture-runtime-secret",
        setting_type = "runtime-global",
        default_value = "preserve-me"
    },
    {
        type = "bool-setting",
        name = "fsm-fixture-hidden",
        setting_type = "startup",
        default_value = true,
        hidden = true,
        forced_value = false
    }
})
