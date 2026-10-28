# Changelog

所有重要变更记录在本文件。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)。

## [Unreleased]

## [0.2.0] - 2026-10-29

### Added

- 宝塔式三步创建向导：数据库信息 → 账号与权限 → 创建结果
- 创建数据库时可一键开放远程访问：自动写入 pg_hba.conf、reload PostgreSQL，失败时保留数据库并返回警告
- 实例连接地址配置：可为每个实例设置对外 IP/域名，生成连接串时优先使用；支持一键填入本机 IP
- 「添加本机 PostgreSQL」快捷入口，自动填入本机连接参数
- 数据库列表默认隐藏系统库，可切换显示
- 账号快速重置密码，自动生成随机密码并可复制
- 创建成功后可直接跳转备份策略配置
- 数据库/账号页无实例时显示引导空状态

### Fixed

- Read-Write 降级为 Read-Only 时未撤销原有写权限
- 对 Owner 数据库执行撤销权限无效果
- 非 `postgres` 超级用户可被误删；当前连接账号可被禁用或降权
- 使用 NOLOGIN 已有账号创建数据库仍返回成功
- 新建空只读库权限显示为「未授权」
- 创建账号时连接数限制 `0` 被忽略
- 连接串未 URL 编码，特殊字符密码、IPv6、Unix Socket 场景不可用；复制脱敏连接串会丢失密码
- Read-Only 账号看不到 Read-Write 账号后续创建的表和序列
- pg_hba.conf 规则字段缺少校验，存在配置注入风险；reload 未使用实例 systemd 服务名
- 删除账号缺少二次确认，依赖错误缺少具体原因

### Changed

- 创建数据库默认权限改为 Read-Write，Owner 放入高级选项并提示风险
- 前端技术术语中文化：Owner / Read-Write / Read-Only / Superuser / CreateDB / CreateRole 等
- 创建账号默认生成随机强密码

## [0.1.1]

### Added

- 账号数据库权限管理：为账号在指定数据库上授予 / 撤销权限（Owner / Read-Write / Read-Only），并展示每个库的真实权限级别
- PostgreSQL 防火墙设置：查看 listen_addresses 与 pg_hba.conf 规则，添加 / 移除来源地址规则并自动 reload
- 账号列表「隐藏系统账号」开关

### Fixed

- 首次注册用户名校验过严（≥3 字符且仅 ASCII），现支持 Unicode、1–64 字符
- 首次注册双重提交导致 `UNIQUE constraint` 报 500（改为优雅返回 `already_initialized`）
- 数据库 / 账号页面首次进入不自动加载（实例自动选中后不刷新）
- 防火墙弹窗布局过窄
- systemd unit 移除 `NoNewPrivileges` 并放行 `/etc/postgresql`，使防火墙管理可用

## [0.1.0]

### Added

- DBPilot V0.1 初始版本
  - PostgreSQL 实例管理（添加/测试连接/编辑/删除/工具检测）
  - 数据库与数据库账号管理
  - 一键创建「数据库 + 独立账号 + 权限」（Owner / Read-Write / Read-Only）
  - 逻辑备份（pg_dump -Fc）与 SHA256 / Manifest
  - 备份目标：本地/挂载目录 与 WebDAV，多目标 + Partial Success
  - 定时备份（每天/每周/每月）+ Retention（Keep Last N）
  - 备份恢复（恢复为新数据库，含 Hash/Manifest 校验）
  - 任务系统与审计日志
  - systemd 部署与 `dbpilot install` 命令
  - 简体中文界面（完整 i18n 架构）、深色/浅色主题
