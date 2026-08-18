# Security Policy

## 报告安全漏洞

如果发现安全漏洞，请通过 GitHub Security Advisory 或邮件私密报告，不要公开披露。

## 安全设计

DBPilot 遵循以下安全原则：

- **凭据加密存储**：PostgreSQL 密码、WebDAV 密码等通过 `SecretStore` 统一加密（AES-GCM），不落明文。
- **Secret 不泄露**：API 不返回明文密码，日志/审计/URL 不携带密码。
- **SQL 安全**：所有标识符使用 `QuoteIdent`，字符串使用 dollar-quoting。
- **命令安全**：`exec.CommandContext` argv 传参，禁用 shell 拼接。
- **Web 安全**：Session Cookie HttpOnly + SameSite，CSRF 防护，登录限流，密码 bcrypt 哈希。
- **路径安全**：本地存储禁止目录穿越。

## 生产部署建议

- 通过反向代理（Nginx/Caddy）启用 HTTPS。
- 不要以 root 运行，使用 `dbpilot install` 创建独立用户。
- 使用独立 PostgreSQL 管理账号，避免直接使用 `postgres` 超级用户（如你的环境允许）。
