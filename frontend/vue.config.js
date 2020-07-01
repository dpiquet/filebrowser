module.exports = {
  runtimeCompiler: true,
  publicPath: '[{[ .StaticURL ]}]',
  configureWebpack: {
    output: {
      filename: '[name].[hash:8].js'
    }
  }
}
