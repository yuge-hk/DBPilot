# Changelog

所有重要变更记录在本文件。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)。

## [Unreleased]

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
