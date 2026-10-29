# Observability contract

Observability is deployed as part of the same Terraform stack as the Lambda
application. Do not introduce a separate managed Grafana workspace for the dev
environment.

- All five Lambda entry points use the shared observability middleware through
  Fx and emit structured request completion logs.
- Request, correlation, Lambda request, W3C trace, and span identifiers must be
  propagated without logging secrets or request bodies.
- Terraform configures CloudWatch filters, dashboard widgets, alarms, active
  X-Ray tracing, and the pinned ADOT layer.
- Dev AWS observability uses CloudWatch Logs, CloudWatch dashboards and alarms,
  and X-Ray directly; no AWS Grafana server is created.
- Local SAM Lambda containers export OTLP over the Compose network to the
  `otel-collector` service in `infra/local`. The collector forwards spans to
  the local Tempo container, while retaining debug logs for quick inspection;
  Grafana is available at `http://127.0.0.1:3000` with provisioned Tempo and
  Loki data sources. Grafana Alloy reads only structured SAM Lambda application
  logs and forwards them to Loki with `service_name` labels for the auth, users,
  posts, ads, and feed Lambdas.
- New Lambda features must add their function to the shared observability
  locals and preserve the standard tracing environment variables.

Validate both Terraform and Go observability contracts before completion.
