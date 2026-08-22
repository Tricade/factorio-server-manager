const path = require('path');
const fs = require('fs');
const webpack = require('webpack');
const MiniCssExtractPlugin = require("mini-css-extract-plugin");
const TerserPlugin = require('terser-webpack-plugin');
const packageMetadata = require('./package.json');

class VersionedIndexPlugin {
    constructor(uiVersion, assetVersion) {
        this.uiVersion = uiVersion;
        this.assetVersion = assetVersion;
    }

    apply(compiler) {
        compiler.hooks.thisCompilation.tap("VersionedIndexPlugin", compilation => {
            compilation.hooks.processAssets.tap(
                {
                    name: "VersionedIndexPlugin",
                    stage: webpack.Compilation.PROCESS_ASSETS_STAGE_ADDITIONAL
                },
                () => {
                    const template = fs.readFileSync(path.resolve(__dirname, 'ui/index.html'), 'utf8');
                    const html = template
                        .replaceAll('__FSM_UI_VERSION__', this.uiVersion)
                        .replaceAll('__FSM_ASSET_VERSION__', this.assetVersion);
                    compilation.emitAsset('index.html', new webpack.sources.RawSource(html));
                }
            );
        });
    }
}

module.exports = (env, argv) => {
    const isProduction = argv.mode === 'production';
    const uiVersion = process.env.FSM_UI_VERSION || packageMetadata.version;
    const uiRevision = process.env.FSM_UI_REVISION || "local";
    const revisionSuffix = ["", "unknown", "local"].includes(uiRevision) ? "" : `-${uiRevision.slice(0, 12)}`;
    const assetVersion = `${uiVersion}${revisionSuffix}`;

    return {
        entry: {
            bundle: './ui/index.js',
            style: './ui/index.scss'
        },
        output: {
            filename: '[name].js',
            path: path.resolve(__dirname, 'app'),
            publicPath: ""
        },
        resolve: {
            alias: {
                Utilities: path.resolve('ui/js/')
            },
            extensions: ['.js', '.json', '.jsx']
        },
        devtool: isProduction ? false : "source-map",
        module: {
            rules: [
                {
                    test: /\.jsx?$/,
                    exclude: /node_modules/,
                    use: {
                        loader: 'babel-loader',
                        options: {
                            presets: [
                                '@babel/preset-env',
                                [
                                    '@babel/preset-react', {
                                        development: !isProduction
                                    }
                                ]
                            ]
                        }
                    }
                },
                {
                    test: /\.scss$/,
                    use: [
                        MiniCssExtractPlugin.loader,
                        {
                            loader: "css-loader",
                            options: {
                                "sourceMap": !isProduction,
                            }
                        },
                        "resolve-url-loader",
                        {
                            loader: "sass-loader",
                            options: {
                                // always make sourceMap. resolver-url-loader is needing it
                                "sourceMap": true,
                            }
                        },
                        {
                            loader: 'postcss-loader',
                            options: {
                                postcssOptions: {
                                    plugins: [
                                        require('tailwindcss'),
                                        require('autoprefixer'),
                                    ],
                                }
                            },
                        }
                    ]
                },
                {
                    test: /(\.(png|jpe?g|gif)$|^((?!font).)*\.svg$)/,
                    use: [
                        {
                            loader: "file-loader",
                            options: {
                                name: loader_path => {
                                    if (!/node_modules/.test(loader_path)) {
                                        return "/images/[name].[ext]?[hash]";
                                    }

                                    return (
                                        "/images/vendor/" +
                                        loader_path.replace(/\\/g, "/")
                                            .replace(/((.*(node_modules))|images|image|img|assets)\//g, '') +
                                        '?[hash]'
                                    );
                                },
                            }
                        }
                    ]
                },
                {
                    test: /(\.(woff2?|ttf|eot|otf)$|font.*\.svg$)/,
                    use: [
                        {
                            loader: "file-loader",
                            options: {
                                name: loader_path => {
                                    if (!/node_modules/.test(loader_path)) {
                                        return '/fonts/[name].[ext]?[hash]';
                                    }

                                    return (
                                        '/fonts/vendor/' +
                                        loader_path
                                            .replace(/\\/g, '/')
                                            .replace(/((.*(node_modules))|fonts|font|assets)\//g, '') +
                                        '?[hash]'
                                    );
                                },
                            }
                        }
                    ]
                }
            ]
        },
        performance: {
            hints: false
        },
        stats: {
            children: false
        },
        plugins: [
            new MiniCssExtractPlugin(),
            new VersionedIndexPlugin(uiVersion, assetVersion),
            new webpack.DefinePlugin({
                __FSM_UI_VERSION__: JSON.stringify(uiVersion),
                __FSM_UI_REVISION__: JSON.stringify(uiRevision)
            })
        ],
        optimization: {
            minimize: isProduction,
            minimizer: [
                new MiniCssExtractPlugin(
                    {
                        filename: "[name].css"
                    }
                ),
                new TerserPlugin()
            ],
        }
    }
}
