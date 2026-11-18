const {test} = require("node:test");
const assert = require("node:assert/strict");
const configure = require("../../webpack.config.js");
const TerserPlugin = require("terser-webpack-plugin");

test("production minifies both stylesheet and JavaScript assets", () => {
    const config = configure({}, {mode: "production"});
    assert.equal(config.optimization.minimize, true);
    const [css, js] = config.optimization.minimizer;
    assert.ok(css instanceof TerserPlugin);
    assert.equal(css.options.minimizer.implementation, TerserPlugin.cssnanoMinify);
    assert.ok(css.options.test.test("style.css"));
    assert.ok(!css.options.test.test("bundle.js"));
    assert.ok(js instanceof TerserPlugin);
    assert.equal(js.options.minimizer.implementation, TerserPlugin.terserMinify);
    assert.ok(js.options.test.test("bundle.js"));
    assert.equal(config.devtool, false);
});

test("production does not serialize minifier options for worker processes", () => {
    const config = configure({}, {mode: "production"});
    for (const plugin of config.optimization.minimizer) {
        assert.equal(plugin.options.parallel, false);
    }
});

test("development retains source maps without minification", () => {
    const config = configure({}, {mode: "development"});
    assert.equal(config.optimization.minimize, false);
    assert.equal(config.devtool, "source-map");
});

test("lazy pages use root-relative content-hashed assets across navigation and updates", () => {
    const config = configure({}, {mode: "production"});
    assert.equal(config.output.publicPath, "/");
    assert.match(config.output.chunkFilename, /^chunks\/.*\[contenthash/);
    assert.equal(config.output.clean, true);
    const imageRule = config.module.rules.find(rule => rule.test.test("brand.png"));
    assert.equal(imageRule.use[0].options.name("ui/assets/brand.png"), "images/[name].[ext]?[hash]");
});
