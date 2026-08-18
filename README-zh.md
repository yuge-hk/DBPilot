# DBPilot

面向 Linux 原生 PostgreSQL 的轻量级、自托管 Web 管理控制台。

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg)](https://go.dev)
[![Vue](https://img.shields.io/badge/Vue-3-42b883.svg)](https://vuejs.org)

DBPilot 把云数据库控制台（AWS RDS、阿里云 RDS、腾讯云数据库）般的简单体验，
带给由你自己运行和拥有的 PostgreSQL。建库、管账号、授权、备份、恢复，
全都在一个干净的 Web 界面里完成。

DBPilot **不是** pgAdmin，不是 SQL IDE，不是 1Panel，不是 Docker 管理器，
也不是 Kubernetes 平台。

## 亮点

- **Linux Native** — 面向自建 PostgreSQL，非容器编排平台
- **Single Binary** — 单个静态 Go 二进制，前端通过 `go:embed` 内嵌
- **No Docker Required** — 直接运行在 Linux，用 `systemd` 管理；运行时无需 Node.js 或 nginx
- **中文优先界面** — 默认 `zh-CN`，完整的 i18n 架构，方便扩展更多语言
- **随处备份** — 本地目录、NFS / SMB 挂载目录、WebDAV（AList、Nextcloud、NAS 等）

## 功能

- PostgreSQL 实例管理，支持连接测试与客户端工具检测
- 数据库与账号（Role）管理，含系统角色保护
- **一键创建** — 数据库 + 独立账号 + 权限模板一步完成，输出连接串，部分失败时安全回滚
- 权限模板：**Owner**、**Read-Write**、**Read-Only**（通过 `ALTER DEFAULT PRIVILEGES` 同时覆盖已有对象与未来对象）
- 逻辑备份：`pg_dump`（自定义 `-Fc` 格式）+ SHA256 + `manifest.json`
- 多备份目标，逐目标记录状态（`success` / `partial_success` / `failed`）
- 定时备份（每天 / 每周 / 每月）与 `Keep Last N` 保留策略
- 恢复到新数据库，带校验和与 Manifest 校验
- 后台任务系统与审计日志

## 支持的 PostgreSQL 版本

| 版本 | 状态 |
|---|---|
| PostgreSQL 16 | 目标 |
| PostgreSQL 17 | 已实测 |
| PostgreSQL 18 | 已实测 |

DBPilot 使用稳定的系统目录（`pg_database`、`pg_roles` 等）与标准 DDL，
因此以 PostgreSQL 16 / 17 / 18 为兼容目标。

> 备份与恢复依赖系统自带的 `pg_dump` / `pg_restore`。请安装与服务器主版本
> 匹配的客户端，否则 DBPilot 会拒绝执行并提示版本不兼容：
>
> ```bash
> apt install postgresql-client-17   # 对应 PostgreSQL 17
> apt install postgresql-client-18   # 对应 PostgreSQL 18
> ```

## 快速开始

```bash
# 安装（创建 dbpilot 用户、目录、systemd unit 并启动）
sudo dbpilot install

# 或直接运行
dbpilot run --config /etc/dbpilot/config.yaml
```

打开 `http://127.0.0.1:8080`，完成管理员初始化，然后：

```text
添加 PostgreSQL 实例（测试连接）
→ 一键创建「数据库 + 独立账号 + Owner 权限」
→ 复制连接串
→ 配置备份目标（/mnt/nas/postgres 或 WebDAV）
→ 设置每天定时备份与保留策略
```

## CLI

```bash
dbpilot version              # 打印版本
dbpilot run                  # 运行服务
dbpilot install [--dry-run]  # 系统用户、目录、配置、systemd unit
dbpilot status               # 运行状态
```

## 开发

后端：

```bash
go build ./...
go test ./...
go vet ./...
```

前端：

```bash
cd web
npm install
npm run typecheck
npm run build
```

集成测试（需要本地 PostgreSQL，默认 `127.0.0.1:5432`、`postgres` / `postgres`）：

```bash
DBPILOT_TEST_PG=1 go test ./internal/provider/postgres/ ./internal/backup/
# 可选：DBPILOT_TEST_PG_PORT=5433 指定非默认端口
```

## 架构

- **后端** — Go（`net/http` + `chi`、`pgx/v5`、`modernc.org/sqlite`、
  `go:embed`、`slog`），无 CGO，仅 Linux
- **前端** — Vue 3 + TypeScript + Vite + Pinia + Vue I18n + Naive UI
- **持久化** — SQLite（仅存 DBPilot 元数据），带版本化 migration
- **凭据** — 通过 `SecretStore`（AES-GCM）集中加密，绝不落入业务表

## 安全

密码不会写入日志、审计，也不会由 API 返回。SQL 标识符始终安全 quote；
外部工具一律 argv 传参、绝不经过 shell。会话使用 HttpOnly + SameSite Cookie，
带 CSRF 防护与登录限流。详见 [SECURITY.md](SECURITY.md)。

## 致谢

DBPilot 的开发得到了 [DeepSeek](https://www.deepseek.com) 与
[OpenCode](https://opencode.ai) 的帮助。

## 贡献

欢迎参与。详见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## License

[Apache-2.0](LICENSE)
