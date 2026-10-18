# Social Media MVP Documentation

This directory contains the product, architecture, API, planning, and operations documentation for the Social Media MVP.

## Authoritative document

[mvp-technical-spec.md](product/mvp-technical-spec.md) is the authoritative MVP implementation contract. If another document conflicts with it, update the other document or explicitly revise the technical specification.

## Documentation map

- [Product](product/): goals, features, and MVP requirements.
- [Architecture](architecture/): Go architecture and DynamoDB design.
- [API](api/): HTTP endpoint contracts and examples.
- [Planning](planning/): epics, user stories, tasks, roadmap, and sprint plans.
- [Operations](operations/): testing, CI/CD, deployment, and operational procedures.

## Documentation rules

- Keep implementation-independent requirements in product documents.
- Keep framework, package, and deployment decisions in architecture or operations documents.
- Keep API request, response, validation, and error contracts in API documents.
- Update the authoritative specification when changing an MVP contract.
