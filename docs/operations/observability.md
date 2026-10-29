# Observability

The dev stack provisions observability together with the application through
the reusable Terraform module.

## Components

- Each Lambda writes structured Zap JSON logs to its own CloudWatch log group.
- CloudWatch metric filters track application and dependency failures.
- A CloudWatch dashboard and alarms cover Lambda, API Gateway, and DynamoDB.
- Lambda active tracing and the pinned ADOT collector layer export sampled
  OpenTelemetry traces to AWS X-Ray.
- Request middleware propagates `X-Request-ID`, `X-Correlation-ID`, and W3C
  `traceparent` values. The same identifiers are included in completion logs.
## AWS observability

AWS Lambda logs are available in the per-function CloudWatch log groups. Lambda
traces are available in AWS X-Ray, and CloudWatch provides the dashboards,
metric filters, and alarms managed by Terraform. No AWS Grafana server is
created for the dev stack.

## Configuration

- `otel_trace_sample_rate` controls the default parent-based trace sampling
  ratio and defaults to `0.1`.
- `log_retention_days` controls Lambda log retention.
- `alarm_actions` accepts an optional list of existing SNS topic ARNs.

Do not log passwords, JWTs, refresh tokens, authorization headers, signing
keys, or raw request bodies. Request and trace identifiers are safe correlation
metadata, not credentials.

## Local traces

Local SAM Lambda containers use the same OTLP HTTP exporter, pointed at the
`otel-collector` service on the Compose network. The collector receives traces
on port `4318`, forwards them to the local Tempo backend, and also writes a
compact summary with its debug exporter:

SAM Lambda application logs are collected by the invoke helper as structured
JSONL in `/tmp/social-media-local-logs/sam-lambda.jsonl`; Alloy tails only that
file and sends it to Loki. Each record is tagged with its Lambda service name:
`auth`, `users`, `posts`, `ads`, or `feed`. DynamoDB, Testcontainers, Docker,
and SAM runtime logs are excluded. Response bodies and tokens are not copied
into the file.

```bash
just local-up
open http://127.0.0.1:3000
docker compose -f infra/local/docker-compose.yaml logs -f otel-collector tempo
```

Log in to Grafana with `admin` / `admin`, then open **Explore** and select the
preconfigured **Tempo** data source. Select **Loki** in Explore to search local
Lambda application logs. The Alloy debugging UI is available at
`http://127.0.0.1:12345`. The local dashboard is available under the `Social
Media` folder. Tempo and Loki retain local data for 24 hours in the named
`social-media-tempo-data` and `social-media-loki-data` volumes.
`just local-down` stops containers but keeps the data;
`docker compose -f infra/local/docker-compose.yaml down -v` also removes the
local Grafana, Tempo, Loki, and Alloy data.

The local collector and Tempo stack is deliberately separate from AWS X-Ray.
The AWS ADOT layer forwards spans to X-Ray in the deployed environment.
