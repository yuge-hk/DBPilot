# Changelog

所有重要变更记录在本文件。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)。

## [Unreleased]

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
