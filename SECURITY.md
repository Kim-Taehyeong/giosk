# Security Policy

## Reporting a vulnerability

Please **do not** open a public issue for security vulnerabilities.

Report privately via GitHub's [Private Vulnerability Reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability)
(the **Security** tab → *Report a vulnerability*). We aim to acknowledge reports within a few
business days and will coordinate a fix and disclosure timeline with you.

## Scope

Giosk provisions real workloads and holds cluster credentials (in-cluster RBAC). Areas of
particular interest:

- Authentication / session handling and the web-terminal WebSocket path
- RBAC scope of the API ServiceAccount
- Multi-tenancy isolation between users / orgs / teams
- The access gateway (token issuance, subdomain routing, SSH proxy)

## Hardening notes for operators

- Set a strong `admin.password` at install; change the default admin credentials on first login.
- Keep the API's ServiceAccount permissions as scoped as your workflows allow.
- Terminate TLS at the frontend / gateway (wildcard cert) for any non-trusted network.
