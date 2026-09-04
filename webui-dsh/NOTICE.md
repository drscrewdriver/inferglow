# NOTICE

本目录(`webui-dsh/`)是 [dsh-transition-webui](https://github.com/drscrewdriver/dsh-transition-webui)
的 vendored 拷贝,在 InferGlow 仓库内整合为独立的浏览器 Web UI 子工程。

- 上游版本:分支 `align-frontend-display`,commit `9e14e99`(2026-09)
- 许可证:MIT(上游 package.json `license` 字段声明;上游仓库未附带 LICENSE 文件)
- 拷贝时排除:`node_modules/`、`dist/`(可重建构建产物)、`INSTALL/CHANGELOG` 的 ja/ko/zh 翻译、
  上游 `.git/` 与编辑器配置
- 本地改造:接入 InferGlow 后端 API(`src/api/` 适配层、mock 切口接线),`vite.config.ts`
  调整 base/outDir/proxy;其余视图层保持上游原样
- 上游更新方式:手工 diff 同步(上游定位为 demo/脚手架,预期演进缓慢)
