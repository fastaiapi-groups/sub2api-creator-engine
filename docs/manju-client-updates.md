# 笑笑漫剧独立在线发布服务

产品 `xiaoxiao-manju`，应用 ID `com.fastaiapi.xiaoxiao.manju`，签名密钥 ID `xiaoxiao-manju-release-v1`。Codex++ 保持原产品、离线签名和发布通道，不修改其私钥。

管理员入口 `/fastaiui/admin/client-releases?product_id=xiaoxiao-manju`，下载页面 `/fastaiui/downloads/manju`。选产品后可创建版本、填写说明、设置强制更新、上传文件、校验签名和发布。上传使用 8 MB 有序分片，重复分片不重复追加。普通用户无发布权限。公开检查和下载不要求登录；下载地址绑定产品、版本、SHA-256 和文件名。

## 无业务停机发布

官网 `/fastaiapi/manju-updates/` 路径由已有 direct-gateway 精确分流到 SERVER32 的独立 4447 端口；去掉此前缀后转发 `/api/client-updates/`、`/downloads/manju/`、`/health`。其他路径、域名、限流与 API 容器保持原状。无需新增域名或 DNS 修改。

`compose.manju-updates.yml` 只启动 `manju-updates-api`。连接原 release PostgreSQL 和 Sub2API 管理员库；管理员认证通过 Docker 网络的 `http://sub2api:8080` 完成，不绕公网。只迁移新增列和上传表，不启动聊天、图片、充值业务。运行配置 `.env.manju-updates` 不加入 Git；数据库地址须用明确容器名 `design-postgres`，避免两个网络的 postgres 别名歧义。

先备份 release 表、客户端文件目录与当前容器元数据，再配置：

- `MANJU_UPDATES_DATABASE_URL`：原发布库连接；保留凭据，只选择明确的 Docker 主机名。
- `MANJU_UPDATES_ADMIN_DATABASE_URL`：原管理员库连接。
- `MANJU_UPDATES_DATA_DIR`：原 client-updates 持久目录；不同产品独立数据库记录与对象路径。
- `MANJU_UPDATES_KEY_FILE`：Git 外的独立 PKCS8 Ed25519 私钥，root 0600，只读挂载；公钥必须与客户端内置公钥一致。

已提交、推送并更新到同一提交后，使用 `sudo ./restart.sh --no-pull --service updates`。该入口不重建业务 API、数据库或原 UI。新服务支持 SIGTERM 时等待下载结束，最长 30 分钟。

前端使用 `sudo ./restart.sh --no-pull --service ui --atomic-static`：先构建新 UI 镜像，再向原容器复制内容摘要资源，最后原子替换 index.html，保留旧资源文件和现有会话，不重启原 UI/API。回退时同样从备份或旧镜像原子发布静态文件；不得全量回滚共享数据库。

## 发布规则

页面 ZIP 验证应用 ID、版本、桌面运行摘要、所有文件大小与哈希，拒绝链接、穿越和解压超限。桌面运行摘要由应用构建生成。仅改页面时上传 ZIP 即可；原生变化须同时上传 macOS arm64 / Windows x64 安装包，否则不兼容客户端会提示缺少匹配安装包。发布记录使用 schema 2，说明、强制标记、文件列表和运行摘要均签名。

检查接口同时返回最近完整安装版本的独立签名，客户端确认其运行摘要与最新页面一致后，先安装兼容版本，再更新到最新页面。页面单独发布时必须与最近完整安装版本的运行摘要一致；否则要求该版本同时提供安装包。

发布按钮前会保存当前说明和强制选项。正式发布必须版本递增，发布后不可改写策略/文件。撤回停止新检查与下载，不删除已下载文件或强制改写客户端版本。公开下载页保留最近一个有效完整安装包，页面包不作为初次安装文件展示。

首次正式版本 0.2.12：默认可选更新，包含完整安装包及页面包。旧应用没有更新代码，需要先手动覆盖安装一次。

## 验证

Go `go test ./...`；发布生命周期集成测试仅接受本机 `TEST_DATABASE_URL`，使用临时 schema 并清理。覆盖现有 Codex 离线发布、产品隔离、管理员角色、强制标记/说明签名、撤回和分片重试。前端 `npx vitest run --config vitest.updates.config.ts` 覆盖策略保存后签名以及普通用户访问控制。不要向生产发布伪造的更高版本以测试弹窗。
