# Contributing

感谢你对 DBPilot 的关注。

## 开发流程

1. Fork 仓库并创建分支。
2. 修改代码，遵循现有代码风格（`gofmt`、TypeScript strict）。
3. 每个 Phase 完成前确保以下检查通过：

```bash
# 后端
gofmt -l .          # 应无输出
go vet ./...
go test ./...
go build ./...

# 前端
cd web
npm run typecheck
npm run build
```

4. 提交并创建 Pull Request。

## 安全约束

- 禁止在日志、审计、URL 中输出任何 Secret。
- SQL 标识符必须使用 `QuoteIdent` / `QuoteLiteral`，禁止字符串拼接。
- 禁止使用 `exec.Command("sh", "-c", ...)`，一律 argv 传参。
- 危险操作（删除数据库/账号/备份）必须有二次确认。

## 提交信息

请使用简洁的英文提交信息，格式参考：

```
feat: add webdav storage backend
fix: prevent path traversal in local storage
```

## License

所有贡献以 [Apache-2.0](LICENSE) 许可发布。
