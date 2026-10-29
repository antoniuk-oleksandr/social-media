# Social Media MVP Documentation

This directory contains the product, architecture, API, planning, and operations documentation for the Social Media MVP.

## Authoritative document

[mvp-technical-spec.md](product/mvp-technical-spec.md) is the authoritative MVP implementation contract. If another document conflicts with it, update the other document or explicitly revise the technical specification.

## Documentation map

- [Product](product/): goals, features, and MVP requirements.
- [Architecture](architecture/): Go architecture and DynamoDB design.
- [API](api/): HTTP endpoint contracts and examples.
- [Development Terraform](../infra/dev/README.md): the initial dev AWS stack and deployment instructions.

## Local infrastructure

Manual local development uses the Compose stack in [`../infra/local/docker-compose.yaml`](../infra/local/docker-compose.yaml). It runs DynamoDB Local on the dedicated Docker network `social-media-local` and publishes it only to `127.0.0.1:8000` on the host.

```bash
docker compose -f infra/local/docker-compose.yaml up -d
docker compose -f infra/local/docker-compose.yaml down
```

Applications running on the host use `http://127.0.0.1:8000`. Applications running in a container attached to `social-media-local` use `http://dynamodb-local:8000`.

Compose is for manual local development. Repository integration tests must use Testcontainers and isolated tables rather than depending on a developer-managed Compose process.
- [Planning](planning/): epics, user stories, tasks, roadmap, and sprint plans.
- [Operations](operations/): testing, CI/CD, deployment, and operational procedures.
- [Observability](operations/observability.md): CloudWatch, X-Ray, and EC2-hosted Grafana operations.

## Documentation rules

- Keep implementation-independent requirements in product documents.
- Keep framework, package, and deployment decisions in architecture or operations documents.
- Keep API request, response, validation, and error contracts in API documents.
- Update the authoritative specification when changing an MVP contract.
