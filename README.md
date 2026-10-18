# Sub2API Creator Engine

为官方 Sub2API 提供独立的绘画、聊天和自动充值扩展。可配合官方 Docker 镜像使用，无需修改官方 Sub2API 源码。

**交流与使用帮助：QQ 群 48878665。**

第一次部署，按「1 → 2 → 3」操作即可。需要自动充值时，再继续看第 4 节。

## 目录

- [1. 部署前准备](#1-部署前准备)
- [2. 首次启动](#2-首次启动)
- [3. 接入主站域名并登录使用](#3-接入主站域名并登录使用)
- [4. 可选：开启自动充值](#4-可选开启自动充值)
- [5. 更新代码与重启](#5-更新代码与重启)
- [6. 日常维护与备份](#6-日常维护与备份)
- [7. 常见问题](#7-常见问题)
- [8. 配置项速查](#8-配置项速查)
- [9. 联系方式](#9-联系方式)

## 1. 部署前准备

请准备：

| 项目 | 说明 |
| --- | --- |
| 一台可运行 Docker 的服务器 | 推荐与现有 Sub2API 部署在同一台服务器，方便接入 |
| 已正常运行的官方 Sub2API | 需要它的站点地址和可正常登录的账号 |
| Git、Docker、Docker Compose | 用于下载代码和启动服务 |
| 主站域名和反向代理配置权限 | 正式使用时，将扩展接到原 Sub2API 域名下 |
| 支付商户、官方数据库和 Redis 连接信息 | 仅在开启自动充值时需要 |

**服务器上不需要单独安装 Go、Node.js、PostgreSQL 或 Redis。** Docker 会构建前后端，并启动绘画功能需要的独立数据库和 Redis。

先在服务器终端确认工具可用：

```bash
git --version
docker --version
docker compose version
```

以上命令应输出版本信息。如果某一条提示 `command not found`，请先安装对应工具。若 Docker 提示无法连接守护进程，请先启动 Docker 服务，并确保当前账号有运行 Docker 的权限。

本指南统一使用 `docker compose`。如果你的服务器安装的是旧版 `docker-compose`，手动运行命令时请替换命令名称；`restart.sh` 会自动兼容两种写法。

## 2. 首次启动

### 第一步：下载代码并进入项目目录

```bash
git clone https://github.com/fastaiapi-groups/sub2api-creator-engine.git
cd sub2api-creator-engine
```

如果你已经下载过项目，直接进入已有目录即可，不需要再克隆一次。

后面的启动、日志和维护命令，都在这个目录里执行。目录内应能看到 `docker-compose.yml`、`restart.sh` 和 `.env.example`。

### 第二步：创建配置文件

```bash
cp .env.example .env
nano .env
```

已有 `.env` 时，直接编辑它，**不要重新复制覆盖原来的配置**。

第一次使用，重点修改以下两项：

```dotenv
# 填写你自己的官方 Sub2API 地址，不要在末尾加 /api 或 /api/v1
FASTAIAPI_BASE_URL=https://你的Sub2API主站域名

# 本扩展独立数据库的密码，请替换默认的 change-me
POSTGRES_PASSWORD=替换为你自己设置的长密码
```

本扩展数据库密码建议使用较长的英文字母和数字组合，避免 `@`、`#`、`:`、`/` 等符号影响连接地址解析。

暂时不用充值功能时，保持：

```dotenv
RECHARGE_ENABLED=false
```

其他配置先用默认值即可：

```dotenv
SUB_ADDS_NAME=fastaidesign
SUB_ADDS_FRONTEND_PORT=4445
SUB_ADDS_BACKEND_PORT=4455
POSTGRES_DB=sub_adds
POSTGRES_USER=sub_adds
```

使用 `nano` 编辑时：按 `Ctrl + O`，再按回车保存，最后按 `Ctrl + X` 退出。

`.env` 保存真实密码和支付密钥，仅留在部署服务器上；`.env.example` 是可以提交到仓库的配置模板。

### 第三步：检查配置并启动

```bash
docker compose config --quiet
docker compose up -d --build
```

第一条命令没有输出，通常表示配置检查通过。第二条命令第一次执行会下载镜像、安装构建依赖，耗时取决于服务器网络。

完成后查看容器状态：

```bash
docker compose ps
```

应能看到 `postgres`、`redis`、`api`、`ui` 四个服务处于运行状态。若某个服务退出或持续重启，请查看它的日志：

```bash
docker compose logs --tail=100 api
docker compose logs --tail=100 ui
docker compose logs --tail=100 postgres
```

也可以在服务器上检查后端健康状态：

```bash
curl http://127.0.0.1:4455/health
```

正常响应中应包含 `"status":"ok"`。这个接口检查的是本扩展的数据库连通性，**不代表支付网关和官方数据库已经联调成功**。

### 第四步：确认页面可以打开

默认页面地址：

```text
http://服务器IP:4445/fastaiui/
```

IP 加端口的地址适合检查页面能否打开。**正式登录使用，请继续配置下一节的主站同域访问**：浏览器不会把另一个域名或端口下的登录信息自动带到这里。

## 3. 接入主站域名并登录使用

假设你的官方 Sub2API 地址是：

```text
https://api.example.com
```

配置完成后，用户通过下面的地址访问扩展：

| 页面 | 示例地址 |
| --- | --- |
| 官方 Sub2API 主站 | `https://api.example.com` |
| 绘画与聊天 | `https://api.example.com/fastaiui/` |
| 自动充值 | `https://api.example.com/fastaiui/recharge` |

### 配置 Nginx 反向代理

打开**现有 Sub2API 主站对应的 Nginx 配置**，把 [nginx.conf.example](nginx.conf.example) 中的 `location` 规则加入该域名的 `server { ... }` 内。

保留主站原来的配置，新增以下两组路径转发：

| 路径 | 默认转发地址 | 作用 |
| --- | --- | --- |
| `/fastaiui/` | `http://127.0.0.1:4445` | 扩展前端页面 |
| `/fastaiapi/` | `http://127.0.0.1:4455/` | 扩展后端接口，末尾斜杠用于移除路径前缀 |

不要把示例文件直接覆盖成主站的完整配置。若原配置已有同名路径，先合并规则，避免重复。

如果 Nginx 直接运行在这台服务器上，修改后执行：

```bash
sudo nginx -t
sudo systemctl reload nginx
```

仅在第一条命令检查通过后执行第二条。如果使用宝塔、其他管理面板或容器版 Nginx，请使用对应面板或容器的检查、重载方式。

> 示例里的 `127.0.0.1` 适用于 Nginx 和本扩展运行在同一台宿主机上的情况。若 Nginx 本身运行在容器里，需使用容器能够访问的地址，不能直接用该容器的 `127.0.0.1` 访问其他容器。

若你修改了 `.env` 中的端口，也要同步修改 Nginx 转发目标。服务器使用同域反向代理后，用户访问主站的 HTTPS 端口即可，不需要直接访问扩展后端端口。

### 用户如何登录

1. 在浏览器中打开原 Sub2API 主站，使用原来的账号登录。
2. 在**同一个浏览器**打开 `https://你的主站域名/fastaiui/`。
3. 开启充值后，打开 `https://你的主站域名/fastaiui/recharge`。

本项目没有单独的注册和登录系统，使用官方主站保存在浏览器中的登录凭证。域名、协议和端口都应与登录时一致。

## 4. 可选：开启自动充值

仅使用绘画和聊天时，可以跳过本节。

自动充值需要：**支付商户配置 + 官方 Sub2API 的 PostgreSQL + 官方 Sub2API 的 Redis**。

### 4.1 先分清两套数据库

| 配置 | 连接哪一套服务 | 用途 |
| --- | --- | --- |
| `POSTGRES_DB` / `POSTGRES_USER` / `POSTGRES_PASSWORD` | 本项目 Compose 创建的独立数据库 | 保存绘画、聊天等扩展数据 |
| `SUB2API_DATABASE_URL` | 官方 Sub2API 正在使用的 PostgreSQL | 增加用户余额、保存充值订单和流水 |
| `SUB2API_REDIS_URL` | 官方 Sub2API 正在使用的 Redis | 刷新官方余额和认证缓存 |

不要把官方数据库的账号密码填写到本项目的 `POSTGRES_*` 中来代替连接配置；那样只会修改本扩展自己的数据库配置。

首次启用前，请备份官方数据库。充值服务会在官方数据库中新建 `sub_adds_recharge_` 前缀的扩展表，并更新用户余额、写入官方充值记录；不会修改官方表结构。

### 4.2 填写支付和连接信息

编辑项目目录的 `.env`：

```dotenv
RECHARGE_ENABLED=true

# 用户访问主站的 HTTPS 地址，不包含 /fastaiui 或 /fastaiapi
RECHARGE_PUBLIC_URL=https://你的Sub2API主站域名

# 支付平台提供的网关地址、商户 PID 和支付对接密钥
RECHARGE_GATEWAY=https://pay.1kexiu.xyz
RECHARGE_PID=你的商户PID
RECHARGE_KEY=你的支付对接密钥

# 官方 Sub2API 实际使用的数据库和 Redis
SUB2API_DATABASE_URL=postgres://用户名:密码@数据库地址:5432/数据库名?sslmode=require
SUB2API_REDIS_URL=redis://:密码@Redis地址:6379/0
```

填写时注意：

- `RECHARGE_PUBLIC_URL` 是浏览器可访问的主站地址，必须为 HTTPS。
- `RECHARGE_KEY` 使用支付商户的**支付对接密钥**，不要使用登录密码或挂机软件密钥。
- 连接地址需要让本扩展的 `api` 容器访问得到；容器里的 `localhost` 指向它自己。
- Redis 地址最后的 `/0` 是数据库编号，应与官方实例一致；若官方使用 `/1`，这里也应填写 `/1`。
- Redis 没有密码时，格式为 `redis://Redis地址:6379/0`；启用 TLS 时根据实际配置使用 `rediss://`。
- 官方数据库账号需要读取用户和 API Key、更新用户余额、写入充值记录，以及创建和维护本扩展表的权限。
- 连接串的用户名、密码中包含特殊符号时，需要对用户名、密码分别进行 URL 编码。
- PostgreSQL 的 `sslmode` 要与服务器实际设置一致。启用 TLS 的数据库可用 `require`；同一受控 Docker 网络里未启用 TLS 的实例需按实际情况使用 `disable`。

### 4.3 官方数据库只在 Docker 内网中时，如何连接

推荐让本扩展的 `api` 容器加入官方数据库所在的 Docker 网络，直接通过容器名称连接。

先查找官方数据库容器名称和网络名称：

```bash
docker ps --format 'table {{.Names}}\t{{.Image}}'
docker network ls
```

找到官方 PostgreSQL 容器后查看它所属的网络，把命令中的示例容器名替换成实际名称：

```bash
docker inspect sub2api-postgres --format '{{json .NetworkSettings.Networks}}'
```

在本项目目录新建 `compose.production.yml`：

```yaml
services:
  api:
    networks:
      - default
      - sub2api

networks:
  sub2api:
    external: true
    name: 替换为官方数据库所在的真实网络名
```

然后将 `.env` 中的连接地址填写为实际的官方容器名称。例如下面假设官方容器名为 `sub2api-postgres` 和 `sub2api-redis`，数据库名为 `sub2api`：

```dotenv
SUB2API_DATABASE_URL=postgres://用户名:密码@sub2api-postgres:5432/sub2api?sslmode=disable
SUB2API_REDIS_URL=redis://:密码@sub2api-redis:6379/0
```

请确认 PostgreSQL、Redis 都能从所加入的网络访问。如果它们位于不同网络，需要把 `api` 分别加入对应网络。

使用两份 Compose 配置启动：

```bash
docker compose -f docker-compose.yml -f compose.production.yml config --quiet
docker compose -f docker-compose.yml -f compose.production.yml up -d --build
```

**后续维护也要带上这两份配置。** 具体命令见第 5、6 节。若你使用云数据库或已有可访问地址，可跳过本小节，直接使用主配置启动。

### 4.4 配置套餐金额

默认有 1、5、10、30、50、100、300、500 八档套餐。默认「支付人民币数字」与「到账美元额度数字」相同，例如支付 ¥5，增加 $5 API 额度。

这是默认商品定价，**不是人民币兑美元的汇率换算**，请根据自己的定价决定是否调整。

需要自定义时，在 `.env` 增加或修改：

```dotenv
RECHARGE_PACKAGES='[{"id":"starter","name":"10 刀额度","amount_cents":7000,"credit":"10"},{"id":"standard","name":"50 刀额度","amount_cents":30000,"credit":"50"}]'
```

这两个示例分别表示：支付 ¥70，到账 $10；支付 ¥300，到账 $50。

| 字段 | 含义 | 示例 |
| --- | --- | --- |
| `id` | 套餐标识，不能重复 | `starter` |
| `name` | 页面显示名称 | `10 刀额度` |
| `amount_cents` | 支付金额，单位为人民币分 | `7000` 表示 ¥70 |
| `credit` | 到账 USD 额度，使用字符串，最多八位小数 | `"10"` 表示 $10 |

保留整行 JSON 外层的单引号。套餐调整只影响新订单，旧订单仍按下单时保存的金额和额度处理。

### 4.5 确认支付回调和到账

用户的操作流程：

1. 登录主站后打开「自动充值」，点击套餐上的「立即充值」。
2. 在确认窗口核对当前充值账号、支付金额、到账额度，选择「支付宝」或「微信支付」。此时尚未向渠道下单。
3. 点击「提交订单」，等待窗口内显示该渠道的支付二维码，用对应 App 扫码。整个过程留在本站，不跳转到支付商页面。
4. 支付完成后窗口自动确认到账并刷新余额；也可点击「我已付款，确认到账」。关闭窗口后，可在「充值记录」点击「继续支付」打开原订单的二维码。

遇到网络超时或二维码暂未显示，点击「重试获取二维码」即可。系统保留原订单并核对渠道状态；已付款时请勿重复扫码。二维码过期后会停止展示，支付异常由管理员核对。


支付回调地址由系统自动生成：

```text
https://你的主站域名/fastaiapi/api/recharge/notify
```

该地址必须能被支付平台从公网访问，不要在这一路径额外添加登录验证或跳转。

配置完成后，重新加载容器中的环境变量：

```bash
# 只使用主配置时
docker compose up -d --build

# 使用官方 Docker 网络附加配置时，改用这一条
docker compose -f docker-compose.yml -f compose.production.yml up -d --build
```

上面两种启动命令**按实际部署方式选一种**执行。

检查流程：

1. 用普通用户账号登录官方主站，再打开充值页，确认套餐价格和到账额度正确。
2. 在确认自己的定价后，选择一个小额套餐，检查弹窗金额和两种支付方式，再提交订单并自行扫码支付。
3. 在「充值记录」核对应付、实付、实际到账额度，在「到账日志」核对余额变更。普通用户只可看自己的订单和日志；「充值管理」仅管理员可见，并默认显示全站订单。
4. 使用官方管理员账号登录，可在同一充值页面看到「充值管理」，查看异常订单、查单补单和退款登记。

服务校验回调签名和平台查单结果，按实付金额自动折算到账，不需要管理员审核。默认 1 元套餐实付 0.01 元，就增加 $0.01 API 额度；未确认付款或实付为零不入账。每个订单只结算一次，重复通知不会重复加余额；金额无效、身份或流水不符仍会拦截。不能以浏览器跳转成功代替到账确认。

**退款需先在支付商户后台完成全额退款，再由管理员在本项目登记凭证并冲正该订单的实际到账余额。** 可用余额不足时转人工处理，目前不支持部分退款。

需要查单、告警、入账事务和退款规则的详细说明，请阅读 [自动充值运行机制与维护](docs/auto-recharge.md)。

## 5. 更新代码与重启

### 普通部署：一条命令更新

进入项目目录后执行：

```bash
./restart.sh
```

脚本会按顺序：

1. 执行 `git pull --ff-only` 拉取代码。
2. 检查 Compose 配置，重新构建镜像并重建容器。
3. 输出当前容器状态。

拉取失败或存在无法快进合并的分支时，脚本立即停止，不会继续重启。更新会重新创建容器，期间页面和接口可能短暂不可用；数据库数据卷会保留。

### 使用了 `compose.production.yml` 的部署

每次更新使用：

```bash
COMPOSE_FILE=docker-compose.yml:compose.production.yml ./restart.sh
```

这样可以保留访问官方数据库所需的附加 Docker 网络。

### 只修改了 `.env`，想重新应用配置

只使用主配置时：

```bash
docker compose up -d --build
```

使用附加生产配置时：

```bash
docker compose -f docker-compose.yml -f compose.production.yml up -d --build
```

选择与你的部署方式一致的一条即可。仅执行 `docker compose restart` 不会重新加载 `.env` 中修改的环境变量。

## 6. 日常维护与备份

以下命令默认只使用主配置。**若使用了附加生产配置**，可以先在当前终端执行：

```bash
export COMPOSE_FILE=docker-compose.yml:compose.production.yml
```

此后本节的 `docker compose ...` 和 `./restart.sh` 都会使用两份配置。新开终端后需要重新设置；若项目不再使用附加配置，用 `unset COMPOSE_FILE` 取消。

| 需求 | 命令 |
| --- | --- |
| 查看服务状态 | `docker compose ps` |
| 查看最近 100 行后端日志 | `docker compose logs --tail=100 api` |
| 持续查看后端日志 | `docker compose logs -f api`，按 `Ctrl + C` 退出查看 |
| 查看所有服务最近日志 | `docker compose logs --tail=100` |
| 临时停止 | `docker compose stop` |
| 启动已停止的服务 | `docker compose start` |
| 停止并移除容器，保留数据卷 | `docker compose down` |
| 重新创建并启动 | `docker compose up -d --build` |

**不要把 `docker compose down -v` 用作日常重启命令，它会删除本项目的数据卷。**

备份本扩展的绘画/聊天数据库：

```bash
docker compose exec -T postgres sh -c 'pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB"' > "sub-adds-backup-$(date +%Y%m%d-%H%M%S).sql"
```

该命令只备份本项目独立的 PostgreSQL。**充值订单、入账日志和用户余额存放在官方 Sub2API 数据库中，需要另行备份官方数据库。** 请将备份文件和 `.env` 妥善保存到安全位置。

已经产生数据后，不要随意修改 `SUB_ADDS_NAME`、`POSTGRES_DB`、`POSTGRES_USER`、`POSTGRES_PASSWORD`。修改项目名可能让 Compose 使用另一套数据卷；修改环境变量也不会自动更改已初始化数据库中的账号密码。

## 7. 常见问题

### 页面打不开

先执行 `docker compose ps`，确认 `ui` 和 `api` 都在运行。如果通过域名访问，检查 Nginx 的 `/fastaiui/` 和 `/fastaiapi/` 转发；如果只是测试 IP 加端口访问，检查 `.env` 的前端端口和服务器访问规则。

### 页面能打开，但提示未登录或 401

先登录原 Sub2API 主站，再从同一浏览器打开主站域名下的扩展。不要用另一个域名、HTTP 协议或 IP 加端口的地址代替。登录过期时，回主站重新登录后刷新扩展页面。

### 绘画初始化失败或模型接口报错

检查 `FASTAIAPI_BASE_URL` 是否指向自己的官方 Sub2API 根地址、末尾是否误加 `/api`。再检查主站账号、API Key、可用分组、余额和所选模型是否正常。具体错误看 `docker compose logs --tail=100 api`。

### 充值页面提示「充值服务尚未配置」或 503

如果 `RECHARGE_ENABLED=false`，充值服务未启用，这是预期行为。需要充值时按第 4 节填写全部配置，并用 `docker compose up -d --build` 重新创建容器；使用附加配置的部署仍需带上附加配置。

### 开启充值后，后端一直重启

查看 `api` 日志，重点检查官方数据库、Redis 地址和账号密码，以及 `RECHARGE_PUBLIC_URL`、商户 PID 和密钥是否填写。连接官方内网数据库时，确认 `api` 已加入正确的 Docker 网络。`localhost` 或 Linux 上未配置的 `host.docker.internal` 通常不能直接用于访问官方容器。

### 提示 `address already in use` 或端口被占用

在 `.env` 中修改 `SUB_ADDS_FRONTEND_PORT`、`SUB_ADDS_BACKEND_PORT`，重新创建容器，并同步修改 Nginx 的目标端口。

### 已付款但没有到账

先查看「充值记录」和「到账日志」，可以点击「我已付款，查到账」。后台也会自动查单补偿。仍未到账时，由管理员进入「充值管理」查看金额差异、查单失败等异常，并核对支付平台订单。请保留订单号，避免重复付款。

### 金额一致仍被拦截

系统同时核对平台查单的 `money` 和 `price`。若通道有加价、抹零、减免，或金额字段缺失，会进入异常核对。请先确认平台金额口径，不要直接强制加余额；详见 [充值维护说明](docs/auto-recharge.md)。

### 更新时提示本地改动会被覆盖或分支分叉

先执行 `git status` 查看原因。保留并处理自己的改动或联系维护人员后再重试，不要直接用强制重置覆盖配置和代码。`restart.sh` 遇到拉取失败会停止，原有容器不会因这一步被重启。

### `restart.sh` 提示没有执行权限

```bash
chmod +x restart.sh
./restart.sh
```

使用附加生产配置时，第二条命令仍应改为 `COMPOSE_FILE=docker-compose.yml:compose.production.yml ./restart.sh`。

## 8. 配置项速查

配置模板见 [.env.example](.env.example)。修改 `.env` 后需重新创建容器才能应用。

| 配置项 | 默认值 / 填写内容 | 说明 |
| --- | --- | --- |
| `FASTAIAPI_BASE_URL` | 你的官方 Sub2API 根地址 | 首次部署必须修改 |
| `SUB_ADDS_NAME` | `fastaidesign` | Compose 项目名和容器名称前缀 |
| `SUB_ADDS_FRONTEND_PORT` | `4445` | 对外前端端口 |
| `SUB_ADDS_BACKEND_PORT` | `4455` | 对外后端端口 |
| `POSTGRES_DB` | `sub_adds` | 本扩展独立数据库名 |
| `POSTGRES_USER` | `sub_adds` | 本扩展独立数据库用户 |
| `POSTGRES_PASSWORD` | `change-me`，部署前请修改 | 本扩展独立数据库密码 |
| `RECHARGE_ENABLED` | `false` | 需要充值时改为 `true` |
| `RECHARGE_PUBLIC_URL` | 主站 HTTPS 根地址 | 开启充值必填 |
| `RECHARGE_GATEWAY` | `https://pay.1kexiu.xyz` | 支付网关地址 |
| `RECHARGE_PID` | 支付商户 PID | 开启充值必填 |
| `RECHARGE_KEY` | 支付对接密钥 | 开启充值必填，仅保存在服务端 |
| `SUB2API_DATABASE_URL` | 官方 PostgreSQL 连接串 | 开启充值必填 |
| `SUB2API_REDIS_URL` | 官方 Redis 连接串 | 开启充值必填，核对末尾数据库编号 |
| `RECHARGE_PACKAGES` | 留空使用八档默认套餐 | 可选的套餐 JSON |
| `RECHARGE_ALERT_WEBHOOK_URL` | 留空 | 可选的 HTTPS 异常告警接收地址 |
| `RECHARGE_ALERT_TOKEN` | 留空 | 可选的告警 Bearer Token |

## 9. 联系方式

**QQ 群：48878665**

遇到问题时，建议说明部署方式、执行的命令、报错信息和相关订单号。发日志或配置截图前，请遮住数据库密码、支付密钥和登录 Token，方便安全地定位问题。
