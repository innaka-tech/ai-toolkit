# Operations: deploy, observe, recover

If it is not monitored it is not in production; if the restore was never tested there is no backup. Optimise for availability, recoverability, and operational simplicity.

## Environments and deploys
- Same artefact promoted through environments (build once); configuration per environment from variables or a secret store.
- Infrastructure and configuration as code in the repository (Docker Compose, Terraform, Ansible, Proxmox templates …); no hand-edited servers.
- Every deploy is automated, repeatable, and has a rollback: previous artefact plus backward-compatible database changes (see `aitk standards database`).
- Health checks (liveness and readiness) and a post-deploy smoke test; `aitk deploy <target>` runs it as the target's post_check.
- Containers: minimal base image, non-root user, pinned tags or digests, resource limits, no secrets in images.

## Observability
- Logs: structured, centralised, correlated by request id, retained per policy.
- Metrics: RED for services (rate, errors, duration) and USE for resources (utilisation, saturation, errors), e.g. Prometheus + Grafana.
- Traces for requests that cross services (OpenTelemetry).
- Alerts on symptoms users feel (error rate, latency, queue age), each with a runbook link and an owner; no alert nobody acts on.
- Service level objectives for critical journeys (e.g. 99.9% of label-print requests under 2 s); error budget guides release pace.

## Backup and disaster recovery
- RPO and RTO per system, agreed with the business; backups automated, encrypted, off-site and immutable (3-2-1); hypervisor snapshots are not backups.
- Restore drills at least quarterly; record the measured RTO and fix the gap.
- Runbooks for: restore, failover, certificate expiry, disk full, credential rotation.

## Change and incident (ITIL-aligned, lightweight)
- Normal changes have a ticket/task with risk, rollback, and verification; emergency changes are reviewed afterwards.
- Incidents: detect → triage (severity) → mitigate → resolve → blameless post-incident review within five working days, with tasks for each contributing cause.

## Hardening
- Hosts follow CIS Benchmarks where practical: patched, minimal services, SSH keys only, firewall default-deny, time synchronised, audit logging on.
- Management interfaces (hypervisor, database, dashboards) on a separate network or VPN, never exposed publicly.
