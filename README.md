# sub-adds

独立的绘画/聊天及自动充值扩展服务。绘画数据使用本项目独立的 PostgreSQL 与 Redis；自动充值另外连接官方 Sub2API 的数据库和 Redis。认证和模型请求通过 `FASTAIAPI_BASE_URL` 调用主项目，浏览器携带的 Bearer token 会由本服务转发。

```bash
cp .env.example .env
docker compose up -d --build
```

项目和容器名称前缀由 `SUB_ADDS_NAME` 控制，默认值为 `fastaidesign`。
默认端口由 `.env` 控制：前端 `SUB_ADDS_FRONTEND_PORT=4445`，后端
`SUB_ADDS_BACKEND_PORT=4455`。统一 Nginx 的参考规则见 `nginx.conf.example`：
`/fastaiui/` 转发前端，`/fastaiapi/` 转发后端。

## 自动充值

独立页面：`https://你的主站域名/fastaiui/recharge`，绘画页面顶部也有「余额充值」入口。
页面提供套餐购买、支付宝/微信支付、继续支付、主动查询到账、最近 100 条个人订单与日志。官方管理员登录后可见「充值管理」。
需要与官方 Sub2API 前端部署在**同一域名**，沿用官方登录后保存在浏览器的 `auth_token`。
用户通过官方 `/api/v1/auth/me` 验证身份，订单的用户 ID、金额和到账额度均由服务端确定。

### 配置与部署

在 `.env` 配置（真实支付密钥不要提交到 Git）：

```dotenv
RECHARGE_ENABLED=true
RECHARGE_PUBLIC_URL=https://你的主站域名
RECHARGE_GATEWAY=https://pay.1kexiu.xyz
RECHARGE_PID=你的商户PID
RECHARGE_KEY=你的商户对接密钥
# 可选：HTTPS 异常告警接收地址及 Bearer token
RECHARGE_ALERT_WEBHOOK_URL=
RECHARGE_ALERT_TOKEN=
SUB2API_DATABASE_URL=postgres://用户:密码@可访问的数据库地址:5432/sub2api?sslmode=require
SUB2API_REDIS_URL=redis://:密码@可访问的Redis地址:6379/0
```

`SUB2API_DATABASE_URL` 和 `SUB2API_REDIS_URL` 必须指向**官方实例实际使用的数据库和 Redis**，
不是本目录 Compose 新建的绘画数据库/Redis；Redis DB 编号也必须一致。
容器里的 `localhost` 指向容器自身。若官方数据库只在 Docker 网络内开放，可通过 Compose override
把本服务的 `api` 加入官方数据库所在的外部网络，然后填写该网络内的服务名，不需要公开数据库端口：

```yaml
# compose.production.yml，替换下面的网络名和 .env 内的数据库服务地址
services:
  api:
    networks: [default, sub2api]
networks:
  sub2api:
    external: true
    name: 你的官方部署网络名
```

```bash
docker compose -f docker-compose.yml -f compose.production.yml up -d --build
# 查看回调异常或缓存重试日志
docker compose logs -f api
```

已有 `nginx.conf.example` 的转发规则即可使用。必须允许支付网关从公网访问：
`https://你的主站域名/fastaiapi/api/recharge/notify`，不应在该回调路径增加登录验证。
支付完成跳回 `/fastaiui/recharge?order=...`；跳回地址本身不会触发入账。
未配置时保持 `RECHARGE_ENABLED=false`，绘画服务仍可正常运行。`api` 容器配置自动重启，启动时数据库/Redis 暂时不可用可在依赖恢复后重试启动。

默认套餐为 5/10/30/50/100/300/500/1000，支付人民币数字与到账 USD 数字相同，
这是按照参考截图设置的商品定价，并非外汇换算。可以自定义每档价格：

```dotenv
RECHARGE_PACKAGES='[{"id":"starter","name":"10 刀额度","amount_cents":7000,"credit":"10"}]'
```

`amount_cents` 是人民币分，`credit` 是到账 USD 的十进制字符串（最多八位小数）。
套餐调整不影响已创建订单，支付使用订单保存的价格和额度。
每个用户一小时内最多创建 20 个待支付订单。

### 入账和日志

所有业务代码均在 `sub-adds` 内，不依赖修改或重新构建官方镜像。
启用后，服务在**官方数据库**中新建带 `sub_adds_recharge_` 前缀的订单和日志表；
订单表、`users.balance` 增量更新、官方 `redeem_codes` 已使用记录、入账日志在同一 PostgreSQL 事务提交。
官方兑换记录中的 `RA...` 号就是充值订单号（32 字符）。不修改官方表结构。
数据库连接账号需要创建这些扩展表/索引，以及读取 `users`/`api_keys`、更新用户余额、写入 `redeem_codes` 的权限。
首次启用前请备份数据库，并用与生产相同版本的官方实例验证字段兼容性。

回调按网关提供的 MD5 规则对原始参数排序验签，拒绝重复参数，并核对 PID、支付状态、金额、
支付方式与订单。首次入账还必须通过服务端主动查单，核对平台 `code=1/status=1`、订单号、流水、
`money` 和 `price`；两个金额必须均与本地应付金额一致，缺失或差异都会暂停入账并生成异常。
平台公开文档未详细解释 `price` 的费率/抹零语义，当前采取严格一致规则；有通道加价、减免或抹零时会进入人工核对，真实联调需确认其口径。
已入账且流水一致的重复通知可直接确认，避免支付平台临时不可用影响重复通知应答。事务行锁保证并发重复通知只到账一次，全局唯一的网关流水号阻止跨订单重复使用。
不依赖浏览器跳回或前端提交金额。付款前创建的订单不自动作废，以接收延迟付款通知。

入账成功后清理官方 `billing:balance:<uid>` 和该用户 API Key 的 `apikey:auth:<SHA256>` 缓存，
并通过 `auth:cache:invalidate` 通知官方实例清理本地认证缓存。
Redis 失败不会回滚已收款入账；订单的 `cache_synced=false` 是持久重试队列，每 10 秒重试，
进程重启后继续处理。缓存任务携带余额变更版本号，避免退款与旧刷新任务并发时漏刷新。回调在数据库提交后返回纯文本 `success`，数据库失败时返回 `fail` 供网关重试。
缓存键和官方表字段依据本仓库对应的 Sub2API 版本，官方升级时应重新验证。

用户在「充值记录」「到账日志」查看自己的记录，运维人员可以在数据库查询完整日志：

```sql
SELECT o.id, o.user_id, o.amount_cents, o.credit, o.status, o.trade_no,
       o.paid_at, o.cache_synced, l.event, l.detail, l.created_at
FROM sub_adds_recharge_orders o
JOIN sub_adds_recharge_logs l ON l.order_id = o.id
ORDER BY l.id DESC;
```

### 查单补偿与异常管理

查单协议依据 [支付平台公开 API 文档](https://pay.1kexiu.xyz/docs/api)，使用服务端 POST `/api.php`，
`act=order/pid/key/out_trade_no` 放在表单正文，密钥不会进入 URL、浏览器或错误响应；禁止跟随重定向。
后台每轮最多查询 5 笔订单，按订单年龄采用 1 分钟、5 分钟、1 小时、1 天的间隔退避；
已入账订单继续核对 7 天，已退款冲正订单停止自动查单。查单调度与缓存重试独立运行，进程重启后从数据库恢复。
用户可在订单旁点「我已付款，查到账」，管理员可申请「查单补单」；同一订单两次主动查询至少间隔 15 秒。
查单失败不会假定未付款或增加余额。`status=2` 表示平台超时，**不是退款**，不据此自动扣回余额。
超时订单不再显示「继续支付」；如果平台后来确认付款，仍可正常接收回调并核对入账。

「充值管理」读取最近 200 条订单/异常。后台会重新检查登录用户在官方 `users` 表中的 `role=admin`、
`status=active` 和未删除状态，不信任浏览器声明的管理员身份。
管理员可查看金额差异、流水冲突、平台状态变化、连续三次查单失败、入账失败和缓存刷新失败，
填写原因后重新查单，或仅标记某条异常已处理。标记处理不会增加余额，也不绕过平台核对；异常再次出现会重新打开。
相同订单/异常类型合并计数，避免重复通知刷屏，所有管理员操作记录操作人 ID、原因和时间。退款请求审计独立保存，余额不足或事务回滚也不会丢失操作依据。

配置 `RECHARGE_ALERT_WEBHOOK_URL` 后，会 POST 以下结构；可选 `RECHARGE_ALERT_TOKEN` 作为 Bearer 鉴权。
不配置时仅在站内显示，不推送外部消息。推送失败持久重试，间隔由 1 分钟逐步增至最多 1 小时。
告警至少投递一次，接收端使用 `(issue_id, generation)` 去重；同一异常关闭后重开会产生新 generation。
不包含支付密钥、签名或原始网关响应。

```json
{"event":"recharge_exception","issue_id":1,"generation":1,"order_id":"RA...","kind":"payment_mismatch","detail":"金额存在差异，暂停入账"}
```

### 退款登记与余额冲正

公开文档没有退款接口；本模块**不会向支付平台发起退款**，也不提供未经核对的强制加款按钮。
管理员必须先在商户后台完成该订单的**全额退款**，再在充值管理里登记退款凭证、原因，并勾选确认。
服务在一个事务内锁定订单、检查余额足够、扣回该订单的全部到账额度，写入官方 `admin_balance` 负数调整记录、
退款凭证和操作日志，并安排缓存刷新。相同订单/凭证重复提交不会重复扣款；退款凭证不能用于多个订单。
原始付款回调在冲正后再次到达，也不会再次加款。
若用户已经消费额度、可用余额不足或用户已删除，整笔冲正不执行，记录异常，交由管理员处理资金与已消费额度；
不自动产生负余额。本版不支持部分退款，也无法从平台文档中的查单状态自动识别退款。

### 验证

完整的临时数据库测试（需要 Docker，自动新建和清理测试容器，不读取生产 `.env`）：

```bash
bash backend/scripts/test-recharge.sh
```

后端本机单元测试：Go 1.22+ 下运行 `go test ./...`。前端：`npm ci && npm run build`。
`internal/recharge/service_test.go` 提供可选真实 PostgreSQL / Redis 集成测试，
使用 `RECHARGE_TEST_DATABASE_URL`、`RECHARGE_TEST_REDIS_URL` 指向一次性的空测试库后运行。
测试会创建模拟官方表，**不可指向生产库**。覆盖验签、金额/商户/支付方式校验、订单归属、
并发重复回调、流水号复用的整笔事务回滚、官方充值记录、缓存清理与 Redis 故障后的重试；
另覆盖平台公开签名向量、主动查单补偿、少付/多付/缺失金额、未知支付状态、HTTP/业务失败、流水数字精度、
禁止携带密钥跟随跳转、告警去重与重试、退款幂等与不足余额、退款缓存并发、官方管理员权限与跨用户访问拦截、
以及旧版扩展表的增量升级。
真实商户联调还需要启用配置、公网回调地址，以及一笔实际小额付款验证。
