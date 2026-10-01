---
description: Run unit tests across all Go microservices workspace modules and Python agent
---

Run tests across the entire workspace:

1. Change directory to `backend/`.
2. Run `go test ./...` to execute Go unit tests across all services in the `go.work` workspace.
3. Change directory to `backend/services/agent-service` and run `pytest`.
4. Report any test failures, logs, or coverage outputs concisely.
