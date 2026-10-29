# Development Terraform

This directory is the development Terraform root. It provisions the currently
implemented development foundation through the reusable module in
`../modules/social-media`:

- one ARM64 `auth` Lambda using `provided.al2023`;
- API Gateway HTTP API with `POST /api/v1/auth/register`;
- users, uniqueness, and refresh-session DynamoDB tables;
- the JWT signing key in SSM Parameter Store as `SecureString`;
- a private, encrypted, versioned application media S3 bucket;
- Lambda IAM permissions and CloudWatch logs.
- CloudWatch dashboards, metric filters, alarms, and active X-Ray tracing.

All currently implemented Lambda entry points, API Gateway routes, tables, and
observability resources are managed together by this Terraform root.

AWS logs are available in CloudWatch Logs and traces are available in X-Ray.

## Bootstrap artifact

Terraform expects the prebuilt archive at `../../build/auth-lambda.zip` by
default. It must contain an ARM64 Lambda `bootstrap` binary. Terraform owns
the function and configuration; GitHub Actions owns later code artifact
updates.

From the repository root, create the local artifact with:

```bash
just build
```

## Terraform state

CI/CD uses an S3 backend with native S3 lockfiles. The state bucket is created
automatically by the local-state bootstrap in `infra/dev/bootstrap`. The
bootstrap bucket has versioning enabled, public access blocked, and encryption
enabled.

The Justfile performs the bootstrap and backend migration automatically:

```bash
just terraform-init-dev
```

`backend.hcl` contains only non-secret backend settings. Do not put backend
credentials, state files, or secrets in it.

The workflow refuses to deploy when the remote state is empty, preventing an
accidental second copy of the development resources.

## GitHub Actions deployment

The workflow in `.github/workflows/ci-cd.yml` runs tests, the 90% coverage
gate, integration tests, Terraform validation, ARM64 Lambda packaging, and a
complete Terraform plan/apply on pushes to `dev`. It uses GitHub OIDC rather
than AWS access keys and deploys all Terraform-managed infrastructure together
with all packaged Lambda functions.

Configure these variables in the GitHub `dev` environment:

- `AWS_REGION`
- `AWS_DEPLOY_ROLE_ARN`

The deploy role needs access to the Terraform state bucket, the deployed AWS
resources, and `ssm:GetParameter` for the existing JWT signing-key parameter.
The signing key is read from SSM at deployment time; it is not stored in GitHub.

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

To destroy both the application stack and the Terraform bootstrap resources:

```bash
just terraform-destroy-dev
```

This destroys the main `dev` resources first, then removes the Terraform state
bucket. The next deployment will bootstrap a new state bucket automatically.

The SSM parameter value is sensitive Terraform state, so protect the local
state file and migrate to an encrypted remote backend before production.
