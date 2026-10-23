# Development Terraform

This directory provisions the currently implemented development foundation:

- one ARM64 `auth` Lambda using `provided.al2023`;
- API Gateway HTTP API with `POST /api/v1/auth/register`;
- users, uniqueness, and refresh-session DynamoDB tables;
- the JWT signing key in SSM Parameter Store as `SecureString`;
- a private, encrypted, versioned application media S3 bucket;
- Lambda IAM permissions and CloudWatch logs.

Posts, advertisements, feed, and their Lambda functions are intentionally not
created until those application features exist.

## Bootstrap artifact

Terraform expects the prebuilt archive at `../../build/auth-lambda.zip` by
default. It must contain an ARM64 Lambda `bootstrap` binary. Terraform owns
the function and configuration; GitHub Actions owns later code artifact
updates.

From the repository root, create the local artifact with:

```bash
just auth-package
```

## Terraform state

This environment currently uses Terraform's local state, matching the
project's existing environment layout. State files are ignored by Git. A
remote backend can be introduced later as a separate state-migration task.

## Secret environment file

Do not put the signing key in Git. Create a local environment file:

```bash
cp .env.example .env
openssl rand -hex 32
```

Replace the example value in `.env` with the generated output, then load it
before running Terraform:

```bash
set -a
source .env
set +a
```

## Plan and apply

Run the Terraform checks and apply the complete development stack:

```bash
terraform fmt -check -recursive
terraform validate
terraform plan -out=tfplan
terraform apply tfplan
```

The SSM parameter value is sensitive Terraform state, so protect the local
state file and migrate to an encrypted remote backend before production.
