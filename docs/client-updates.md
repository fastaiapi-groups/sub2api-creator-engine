# FastAiApi 客户端发布

生产后台：<https://fastaiapi.cloud/fastaiui/admin/client-releases>（沿用平台管理员登录）。公开下载：<https://fastaiapi.cloud/fastaiui/downloads/codexplus>。官网首页与客户端下载页读取同一已发布记录。

## 发布流程

1. 在 CodexPlusPlus 仓库更新版本号、测试并构建。Windows 使用该仓库 `FastAiApi client installers` GitHub Actions 构建工作流；只取构建产物，用户下载不使用 GitHub。工作流会实际安装旧包、覆盖新包、重复安装并检查配置和 Windows 系统凭据保留，验收成功后才上传候选安装包。
2. macOS 按 AGENTS.md 构建固定本地 bundle，再运行 `node scripts/package-fastaiapi-macos.mjs`。当前仅提供 arm64；不要勾选 Intel 或 universal，除非实际构建对应安装包。
3. 后台创建版本草稿，填写说明并勾选本次必须提供的平台，逐个平台上传包。版本号不可复用，选择的平台必须齐备。
4. 导出待签名 JSON。在 CodexPlusPlus 仓库运行：

   ```sh
   go run scripts/client-release.go sign --key /Users/atom/.config/fastaiapi/client-updates/release-v1.pem --manifest client-release-manifest.json --out signed.json
   ```

5. 上传 `signed.json` 校验。后台核对签名、草稿、文件长度和 SHA-256；显示“校验通过”后点击“发布到官网与客户端”。私钥不要上传网站、提交 Git 或输出日志。
6. 查看下载页，并执行 `cargo run -p codex-plus-core --example check_fastaiapi_update -- <上一版本>` 验证真实客户端验签。下载包对比 SHA-256，再在对应平台实测。

用户的旧版仍指向原作者更新源时，必须先从官网覆盖安装最新版接管版。之后客户端只检查本站，按平台选择安装包，先验签、再下载和校验 SHA-256；不存在原作者源回退。安装身份与既有数据目录保持一致。Windows 会话持久化从 1.11.4 开始支持；旧版没有保存过的内存会话需要首次重新登录，此后在系统凭据库中保留。

## 撤回与修复

后台填写原因后撤回，可停止当前更新检查及该版本新的下载请求。已开始下载或已安装的用户不能远程撤销；也不会自动降级。发布更高版本号的修复包。发布要求版本号和序列高于历史发布版本，撤回不会清除防回放历史。

当前包为 Windows x64、macOS Apple 芯片。1.11.4 已通过 Windows CI 实际安装/覆盖/重装 14 项检查，Mac 两版真实 DMG 替换与系统钥匙串保留测试通过。macOS 当前是本地签名，尚未 Apple 公证。数字清单签名与操作系统代码签名是独立机制。

## 服务与备份

生产目录 `/opt/fastaiapi-groups/sub2api-creator-engine`。更新功能通过 `CLIENT_UPDATES_ENABLED=true` 显式启用；`CLIENT_UPDATES_PUBLIC_KEY` 只放公钥，包目录通过 `CLIENT_UPDATES_DIR` 配置。

生产启动保留三个 compose 文件（避免重建后丢失功能开关与持久化挂载）：

```sh
docker-compose -f docker-compose.yml -f compose.production.yml -f compose.client-updates.yml up -d api ui
```

备份 PostgreSQL 中 `client_releases`、`client_release_channels`、`client_release_audits`，同时备份生产 `client-updates/` 安装包目录、公钥和三个 compose 配置；私钥单独离线备份。包文件不在数据库中，恢复时两者须一致。

第一次部署备份位于服务器 `/root/fastai-client-update-backup-20261024T115919Z`，含源文件快照及原镜像标签。升级数据库表仅新增，无破坏性迁移。撤回保留文件和审计，不执行物理删除。

`/sub-adds-release` 是容器内运维命令，使用容器已有数据库连接，审计 actor=0；常规发布优先使用后台，不导出用户 Token。完整检查接口 `/fastaiapi/api/client-updates/v1/check` 返回签名 envelope；无当前版本返回 204。安装包 URL 固定本站域名，支持 HEAD/Range，未发布与撤回包不可下载。
