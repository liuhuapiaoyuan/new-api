---
name: publish
description: >-
  Use when the user runs /publish, or asks to publish / release / 发布 /
  打包推送 / build-and-push the Docker image for this project.
disable-model-invocation: true
---

# /publish

读取服务器配置，然后打包 Docker 并推送。

## When to run

Only when the user explicitly asks (`/publish`, “发布”, “打包推送”, etc.).
Do **not** auto-run from ambient context.

Default scope stops at **push**. Do **not** SSH/redeploy unless the user
explicitly asks to update the server after push.

## Workflow

Copy and track:

```
Publish Progress:
- [ ] 1. Read server config
- [ ] 2. Confirm image target
- [ ] 3. Build & push
- [ ] 4. Report result
```

### 1. Read server config

Read `docs/服务器.md` first. Extract at least:

| Field | Source in doc |
| --- | --- |
| Domain | 域名 |
| Compose dir | Compose 目录 |
| Image | 业务镜像 |
| SSH / host | SSH |
| Update commands | 发版 / 更新镜像 |

If the doc is missing or the image name is unclear, stop and ask — do not guess
another registry.

### 2. Confirm image target

Production image from the server doc should match the release script:

- Image: `docker.cnb.cool/qzsyzn/docker/newapi`
- Tags: `latest` + timestamp version from `docker/release.sh`

If `docs/服务器.md` image ≠ `docker/release.sh` `BACK_IMAGE_NAME`, stop and
ask which one to use before building.

### 3. Build & push

From the **repo root** (not `docker/`):

```bash
bash docker/release.sh
```

Notes:

- Script writes a timestamp into `VERSION`, then builds with `./Dockerfile`.
- Requires local Docker login to `docker.cnb.cool` already working.
- Build can take several minutes; set a high enough command wait.
- On Windows, prefer Git Bash / WSL for the script; if only PowerShell is
  available, run the equivalent commands from `docker/release.sh` in order
  (write `VERSION`, `docker build`, `docker push` for both tags).

Do **not** amend git just because `VERSION` changed. Leave `VERSION` dirty
unless the user asks to commit it.

### 4. Report

Reply with:

1. Image + tags pushed (quote the script’s final `Released ...` line)
2. Image target confirmed from `docs/服务器.md`
3. Optional next step (only mention, do not run unless asked):

```bash
ssh root@192.168.1.3
cd /www/wwwroot/ai-api.qzsyzn.com/newapi
docker compose pull new-api
docker compose up -d --no-deps --force-recreate new-api
```

Use the SSH/compose paths actually read from `docs/服务器.md`, not hard-coded
values if the doc has been updated.

## Optional: deploy after push

Only if the user explicitly asks to deploy/update the server after publish:

1. SSH using the host from `docs/服务器.md`
2. Run the doc’s 发版 commands for `new-api` only (`--no-deps`)
3. Verify with `docker compose ps` and/or `curl` against local port / `/api/status`

## Failure rules

- Config unreadable / image mismatch → stop, ask
- Docker build fails → show the error; do not push a partial/old tag as success
- Push auth fails → tell user to login to `docker.cnb.cool`, then retry push only
  if the local image tags already exist
