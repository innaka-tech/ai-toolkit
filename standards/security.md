# Security

Security is a property of every change, not a phase. Baseline: OWASP Top 10 (2021) and OWASP ASVS 4.0 level 2 for web and APIs; OWASP Top 10 for LLM applications for AI features; CIS Benchmarks for hosts and containers. Never claim compliance with a standard without evidence for each control.

## Risk analysis (required before building strict work; short form for the rest)
Record in the task's Risk section: impact surface (modules, data, users), threats (STRIDE: spoofing, tampering, repudiation, information disclosure, denial of service, elevation of privilege), mitigations, rollback, validation. An unverified high-impact assumption blocks implementation.

## Controls every change keeps
- **Input**: validate at the trust boundary with an allow-list (type, length, range, format); reject, do not sanitise silently. Encode output for its context (HTML, SQL, shell, URL).
- **Injection**: parameterised queries or an ORM's bound parameters only; no string-built SQL, shell commands, or LDAP/XPath queries from user input.
- **Authentication**: use the platform's or a proven library's implementation; passwords hashed with Argon2id, bcrypt, or scrypt; MFA for admin access; rate-limit and lock out credential endpoints; sessions rotate on login and expire.
- **Authorisation**: deny by default; check on the server for every request and every object (no IDOR); roles and permissions (RBAC) in one place, never in the UI only; least privilege for users, services, and database accounts.
- **Secrets**: from a secret store or environment, never in code, logs, URLs, or the repository; rotate on exposure (`aitk audit` scans for them).
- **Data**: TLS 1.2+ in transit; encrypt sensitive data at rest; collect only what the purpose needs; personal data has a retention period and a deletion path.
- **Dependencies**: lockfile committed; `aitk audit` clean or each finding accepted with a reason and a date.
- **Errors and logs**: no stack traces or internal details to users; security events (login, permission change, failed authorisation) logged with who, what, when, from where — without secrets.
- **Files and uploads**: check type by content, cap size, store outside the web root with generated names, scan where malware is a risk.
- **HTTP**: secure cookies (Secure, HttpOnly, SameSite), CSRF protection for cookie sessions, security headers (CSP, HSTS, X-Content-Type-Options), CORS allow-list, no wildcard with credentials.

## Severity (for findings)
| Severity | Meaning | Response |
|---|---|---|
| Critical | Exploitable remotely without authentication, or data loss/exposure at scale | Stop; fix before anything else ships |
| High | Exploitable with low privilege or user interaction; integrity of important data | Fix in the current release |
| Medium | Needs unusual conditions; defence-in-depth gap | Task in the backlog with a date |
| Low | Hardening, best practice | Task or accepted risk |

## Incident
Contain (revoke, isolate), preserve evidence, fix the root cause, notify as the law and contracts require, then write a blameless post-incident review with tasks.
