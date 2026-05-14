# 14 - Blue-Green Deployment

## What is Blue-Green Deployment?

Blue-Green deployment keeps two identical production environments ("Blue" and "Green").
At any time only one environment is "live" (receiving traffic). The other is idle,
ready to be updated.

To deploy a new version:
1. Deploy the new version to the **idle** environment (no risk — no traffic goes there)
2. Run smoke tests against the idle environment
3. If tests pass, **switch traffic** to it (becomes the new live)
4. The old live environment is now idle — acts as instant rollback if needed

```
  BEFORE DEPLOYMENT:
  ┌──────────┐                ┌──────────────────┐
  │  Client  │────traffic────►│  BLUE (v1 live)  │
  └──────────┘                └──────────────────┘
                              ┌──────────────────┐
                              │  GREEN (idle)    │
                              └──────────────────┘

  AFTER DEPLOY v2 TO GREEN + SWITCH:
  ┌──────────┐                ┌──────────────────┐
  │  Client  │────traffic────►│  GREEN (v2 live) │
  └──────────┘                └──────────────────┘
                              ┌──────────────────┐
                              │  BLUE (v1 idle)  │  ← instant rollback
                              └──────────────────┘
```

---

## Benefits

| Feature | Description |
|---------|-------------|
| Zero downtime | Traffic switches instantly |
| Instant rollback | Previous version is always running |
| Safe testing | Test new version before any traffic hits it |
| Confidence | Production deploy is just a traffic switch |

---

## The Smoke Test Gate

Before switching traffic, the router runs smoke tests against the idle environment:
- `GET /health` must return 200
- Response must contain `{"status":"ok"}`

If smoke tests fail, the switch is aborted — the live environment stays live.

---

## File Structure

```
14-blue-green/
├── README.md
├── cmd/
│   └── main.go                         ← demo: deploy v2, smoke test, switch traffic
├── internal/
│   ├── env/
│   │   └── environment.go              ← Environment: start, stop, version, health check
│   └── router/
│       └── router.go                   ← Router: tracks live/idle, switches traffic, smoke tests
└── blue_green_test.go                  ← tests for switching, rollback, smoke test gate
```
