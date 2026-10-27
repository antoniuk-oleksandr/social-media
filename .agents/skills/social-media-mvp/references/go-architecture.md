# Go Project Architecture

> MVP authentication, validation, feed, infrastructure, CI/CD, and acceptance contracts are defined in [mvp-technical-spec.md](../product/mvp-technical-spec.md).

## Architectural approach

The application uses a feature-oriented layered architecture inspired by Clean Architecture and Hexagonal Architecture. Each feature is split consistently across the same layers, while shared infrastructure remains separate.

The business domain must remain independent from:

- AWS Lambda and API Gateway
- Chi and HTTP-specific concerns
- DynamoDB and AWS SDK types
- JWT libraries
- bcrypt implementation details
- Logging, tracing, and other infrastructure frameworks

Dependencies point inward toward the domain and application layers.

```text
AWS Lambda / algnhsa
        ↓
Chi router / HTTP controllers
        ↓
DTOs and request validation
        ↓
Application services
        ↓
Domain entities, policies, and errors
        ↑
Repository interfaces
        ↑
DynamoDB repository implementations
```

The domain and service layers define interfaces. Infrastructure packages implement those interfaces. The application is composed in each Lambda entry point.

## Configuration and AWS SSM Parameter Store

Application configuration follows an environment-based Go configuration approach. Feature packages receive typed configuration through constructors and must not read environment variables or SSM parameters directly.

### Local DynamoDB configuration

Manual local development uses the Compose service in `infra/local/docker-compose.yaml`.

```text
Host process:       DYNAMODB_ENDPOINT=<published-local-dynamodb-endpoint>
Docker network:     DYNAMODB_ENDPOINT=http://dynamodb-local:8000
Docker network:     social-media-local (dedicated local-development network)
```

The local endpoint is a non-secret development override. Production configuration must continue to use AWS DynamoDB and must not use the local endpoint. Integration tests use Testcontainers rather than requiring this Compose stack to be running.

Configuration responsibilities are split as follows:

- `internal/platform/config` defines typed configuration structs, environment variable names, parsing, defaults, and validation.
- `internal/platform/ssm` provides an adapter for AWS Systems Manager Parameter Store.
- `cmd` or the Fx composition root loads configuration before constructing feature services.
- Feature packages receive configuration values or typed configuration objects through constructors.

Use environment variables for non-secret runtime configuration and for SSM parameter names or prefixes. Store sensitive values such as JWT signing keys and feed-cursor signing keys in SSM Parameter Store as `SecureString` parameters. Opaque refresh tokens are stored only as SHA-256 hashes in DynamoDB.

Example environment variables:

```text
APP_ENV=prod
AWS_REGION=eu-central-1
SSM_PARAMETER_PREFIX=/theads/prod
JWT_ACCESS_SECRET_PARAMETER=/theads/prod/auth/access-secret
ADS_CURSOR_KEY_PARAMETER=/theads/prod/ads/cursor-signing-key
USERS_TABLE_NAME=theads-users
POSTS_TABLE_NAME=theads-posts
ADS_TABLE_NAME=theads-ads
```

The configuration loader should:

1. Read environment variables.
2. Parse values into typed configuration.
3. Resolve configured SSM parameters through the SSM adapter.
4. Validate required values and ranges.
5. Return an immutable configuration object to the composition root.

Example configuration port:

```go
type ParameterStore interface {
	Get(ctx context.Context, name string, decrypt bool) (string, error)
}
```

The SSM implementation may use `ssm:GetParameter` with decryption enabled for `SecureString` values. SSM and AWS SDK errors must be translated into startup configuration errors and must never be exposed to HTTP clients.

Configuration loading should happen once during cold-start initialization and the resulting configuration should be reused by the warm Lambda container. Do not fetch SSM parameters on every request unless a deliberate runtime refresh policy is introduced.

Environment variables may override non-secret local-development values. Production secrets must come from SSM or another explicitly approved secret source, never from committed files, logs, or hard-coded defaults.

## Interfaces, implementations, and constructors

Use interfaces for feature ports and unexported structs for their implementations. Constructors wire dependencies and return the interface expected by the caller.

The constructor pattern applies to repositories, services, handlers, and other replaceable adapters.

```go
type UserRepository interface {
	CreateUser(ctx context.Context, user user.User) error
	GetByID(ctx context.Context, id user.ID) (user.User, error)
}

type userRepository struct {
	client    *dynamodb.Client
	tableName string
}

func NewUserRepository(client *dynamodb.Client, tableName string) UserRepository {
	return &userRepository{
		client:    client,
		tableName: tableName,
	}
}
```

The concrete `userRepository` type remains private to the implementation package. Services depend on `UserRepository`, not on `userRepository`.

The same pattern is used for services:

```go
type UserService interface {
	Register(ctx context.Context, command RegisterCommand) (User, error)
	GetByID(ctx context.Context, id ID) (User, error)
}

type userService struct {
	repository UserRepository
	clock      Clock
}

func NewUserService(repository UserRepository, clock Clock) UserService {
	return &userService{
		repository: repository,
		clock:      clock,
	}
}
```

This keeps dependency injection explicit and makes unit tests easy to assemble with fakes:

```go
service := NewUserService(&fakeUserRepository{}, fakeClock)
```

Constructors should validate required dependencies and panic only during application startup for impossible configuration errors. Runtime failures should be returned as errors from methods.

## Uber Fx dependency injection

Uber Fx is used as the composition-root dependency injection framework. Feature packages expose normal Go constructors; Fx wires those constructors together in the Lambda entry points.

Feature packages should not call `fx.New` themselves. Fx-specific wiring belongs in `cmd` or an infrastructure package such as `internal/platform/fx`, keeping feature logic easy to test without a container.

Example feature constructors:

```go
func NewUserRepository(client *dynamodb.Client, tableName string) UserRepository {
	return &userRepository{client: client, tableName: tableName}
}

func NewUserService(repository UserRepository, clock Clock) UserService {
	return &userService{repository: repository, clock: clock}
}

func NewHandler(service UserService) *Handler {
	return &Handler{service: service}
}
```

The command package composes them:

```go
func main() {
	var handler http.Handler

	app := fx.New(
		fx.Provide(
			config.Load,
			NewDynamoDBClient,
			user.NewUserRepository,
			user.NewUserService,
			user.NewHandler,
			user.NewRouter,
		),
		fx.Populate(&handler),
	)

	ctx := context.Background()
	if err := app.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer app.Stop(ctx)

	lambda.Start(algnhsa.New(handler))
}
```

`fx.Provide` registers constructors and Fx resolves dependencies from constructor parameters and return types. `fx.Populate` requests the application root so the required components are constructed. For Lambda, use `Start` and `Stop` around the Lambda handler instead of `app.Run`, because the Lambda runtime owns the process lifecycle.

Use Fx lifecycle hooks only for resources that need startup or shutdown behavior, such as database clients, background workers, or telemetry exporters. Constructors should remain side-effect-light.

## Recommended project structure

```text
.
├── cmd/
│   ├── auth/
│   │   └── main.go
│   ├── users/
│   │   └── main.go
│   ├── posts/
│   │   └── main.go
│   ├── ads/
│   │   └── main.go
│   └── feed/
│       └── main.go
│
├── internal/
│   ├── auth/
│   │   ├── domain.go
│   │   ├── dto.go
│   │   ├── mapper.go
│   │   ├── service.go
│   │   ├── handler.go
│   │   └── router.go
│   ├── user/
│   │   ├── user.go
│   │   ├── dto.go
│   │   ├── mapper.go
│   │   ├── service.go
│   │   ├── handler.go
│   │   ├── router.go
│   │   ├── repository.go
│   │   └── dynamodb/
│   │       ├── model.go
│   │       ├── mapper.go
│   │       └── repository.go
│   ├── post/
│   │   ├── post.go
│   │   ├── dto.go
│   │   ├── mapper.go
│   │   ├── service.go
│   │   ├── handler.go
│   │   ├── router.go
│   │   ├── repository.go
│   │   └── dynamodb/
│   │       ├── model.go
│   │       ├── mapper.go
│   │       └── repository.go
│   ├── ad/
│   │   ├── ad.go
│   │   ├── dto.go
│   │   ├── mapper.go
│   │   ├── service.go
│   │   ├── handler.go
│   │   ├── router.go
│   │   ├── repository.go
│   │   └── dynamodb/
│   │       ├── model.go
│   │       ├── mapper.go
│   │       └── repository.go
│   ├── feed/
│   │   ├── feed.go
│   │   ├── dto.go
│   │   ├── mapper.go
│   │   ├── service.go
│   │   ├── handler.go
│   │   ├── router.go
│   │   ├── repository.go
│   │   └── dynamodb/
│   │       ├── model.go
│   │       ├── mapper.go
│   │       └── repository.go
│   ├── platform/
│   │   ├── config/
│   │   ├── fx/
│   │   ├── ssm/
│   │   ├── jwt/
│   │   ├── password/
│   │   ├── clock/
│   │   ├── requestid/
│   │   └── logging/
│   └── shared/
│       ├── errors.go
│       ├── middleware.go
│       └── response.go
│
├── migrations/
├── go.mod
└── go.sum
```

`internal` prevents other Go modules from importing application implementation packages. Each feature package owns its business logic, HTTP adapter, DTOs, mappers, repository port, and infrastructure adapter. `cmd` contains only composition and Lambda entry-point code; it should not contain business logic.

## Feature package responsibilities

### Domain definitions inside each feature

Each feature keeps its domain definitions at the package root. For example, `internal/auth/domain.go` or `internal/user/user.go` defines the feature's entities, domain errors, and repository interfaces.

- User, post, advertisement, and feed entities
- Status values and domain invariants
- Domain errors
- Repository and service ports required by the business logic
- Feed composition policy

The domain layer must not import:

- `aws-sdk-go`
- `chi`
- `algnhsa`
- `net/http`
- DynamoDB attribute-value types
- JSON or HTTP DTOs

Domain entities are plain Go business structures. They must not contain DynamoDB tags, DynamoDB key expressions, AWS SDK types, JSON tags, or HTTP concerns.

Example:

```go
// internal/user/user.go
package user

var (
	ErrNotFound       = errors.New("user not found")
	ErrEmailTaken     = errors.New("email already used")
	ErrUsernameTaken  = errors.New("username already taken")
)

type User struct {
	ID          string
	Email       string
	Username    string
	DisplayName string
	Role        string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Repository interface {
	GetByID(ctx context.Context, id string) (User, error)
	GetByEmail(ctx context.Context, email string) (User, error)
	GetByUsername(ctx context.Context, username string) (User, error)
	Create(ctx context.Context, value User) error
	Update(ctx context.Context, value User) error
}
```

The same pattern applies to `auth`, `post`, `ad`, and `feed`.

### Application service layer

Services coordinate use cases. They should be organized around business operations rather than database tables.

Examples:

- `RegisterUser`
- `Login`
- `RefreshToken`
- `UpdateProfile`
- `CreatePost`
- `DeletePost`
- `CreateAdvertisement`
- `BuildFeed`

Services are responsible for:

- Validating business rules
- Calling repository and infrastructure ports
- Enforcing ownership and authorization decisions
- Coordinating transactions where required
- Translating repository results into domain results
- Applying the feed policy

Services must accept and return domain entities, value objects, or domain-level commands/results. They must never accept or return HTTP requests, HTTP DTOs, JSON transport structures, DynamoDB persistence models, AWS SDK types, Chi types, or Lambda types.

The boundary flow is:

```text
HTTP body
  → handler decodes DTO
  → feature mapper creates domain command/entity
  → service operates only on domain types
  → repository returns domain entity
  → DynamoDB adapter maps the entity to a persistence model
  → DynamoDB stores the persistence model
```

Example:

```go
type UserService interface {
	Register(ctx context.Context, command user.RegisterCommand) (user.User, auth.TokenPair, error)
	Login(ctx context.Context, command auth.LoginCommand) (user.User, auth.TokenPair, error)
	GetByID(ctx context.Context, userID user.ID) (user.User, error)
	UpdateProfile(ctx context.Context, command user.UpdateProfileCommand) (user.User, error)
}
```

The command types are domain/application contracts defined in the relevant feature package. They are not HTTP DTOs.

### Controller layer

Controllers adapt HTTP requests to application service calls.

Controllers are responsible for:

- Reading route parameters, headers, query parameters, and JSON bodies
- Decoding DTOs
- Performing transport-level validation
- Calling services
- Mapping domain results to response DTOs
- Mapping domain errors to HTTP status codes
- Writing consistent JSON responses

Controllers must not:

- Call DynamoDB directly
- Construct DynamoDB expressions
- Contain feed business rules
- Decide whether a user owns an entity
- Expose AWS SDK errors to clients

### DTO layer

DTOs are transport structures and should contain JSON tags only.

```go
type CreatePostRequest struct {
	Content string `json:"content"`
}

type PostResponse struct {
	ID        string `json:"id"`
	AuthorID  string `json:"authorId"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
}
```

DTOs should not be reused as domain or DynamoDB models. Explicit conversion functions make boundary changes visible and prevent storage fields from leaking through the API.

Controllers should use DTO mappers rather than manually copying fields in every handler.

## HTTP request-body validation

Request-body validation belongs at the HTTP boundary, inside the feature handler before the service is called.

Each request DTO should define the transport contract with JSON and validation tags:

```go
type RegisterRequest struct {
	Username    string `json:"username" validate:"required,min=3,max=32"`
	Email       string `json:"email" validate:"required,email"`
	Password    string `json:"password" validate:"required,min=8,max=128"`
	DisplayName string `json:"displayName" validate:"required,max=100"`
}
```

Handlers should:

1. Limit the request body size.
2. Decode JSON with a decoder configured to reject unknown fields.
3. Reject malformed JSON and trailing JSON values.
4. Validate DTO tags and return a consistent `VALIDATION_ERROR` response.
5. Map the valid DTO to an application input using a mapper.
6. Call the service for business validation and authorization rules.

Transport validation should cover shape and basic constraints. Service validation should cover business rules such as ownership, allowed status transitions, duplicate values, and feed cursor validity. Validation errors must never expose internal DynamoDB or AWS SDK details.

### Persistence model layer

Each feature's `dynamodb/model.go` contains persistence models only. These are DynamoDB-specific structs with `dynamodbav` tags. They are not domain models and must not be placed in the feature root alongside domain entities.

```go
type UserRecord struct {
	UserID       string `dynamodbav:"userId"`
	Email        string `dynamodbav:"email"`
	Username     string `dynamodbav:"username"`
	PasswordHash string `dynamodbav:"passwordHash"`
	Role         string `dynamodbav:"role"`
	Status       string `dynamodbav:"status"`
	CreatedAt    string `dynamodbav:"createdAt"`
	UpdatedAt    string `dynamodbav:"updatedAt"`
}
```

Persistence models must remain inside the feature's `dynamodb` boundary. Only the feature's DynamoDB adapter should import its `model.go`. They must not be returned by services or handlers.

## Mappers

Mappers are explicit boundary adapters between DTOs, domain entities, service contracts, and persistence models. They prevent transport and database representations from leaking into the business layer.

Recommended mapper directions:

```text
HTTP request DTO  →  application input
application result →  HTTP response DTO

DynamoDB record   →  domain entity
domain entity     →  DynamoDB record
```

Recommended placement:

- `<feature>/mapper.go` for DTO and domain/application conversions.
- `<feature>/dynamodb/mapper.go` for DynamoDB record and domain conversions, because persistence mapping belongs to the database adapter.

Example:

```go
func ToCreatePostCommand(value dto.CreatePostRequest) post.CreateCommand {
	return post.CreateCommand{Content: value.Content}
}

func ToPostResponse(value post.Post) dto.PostResponse {
	return dto.PostResponse{
		ID:        value.ID,
		AuthorID:  value.AuthorID,
		Content:   value.Content,
		CreatedAt: value.CreatedAt.Format(time.RFC3339),
	}
}
```

The DynamoDB adapter owns conversions such as `user/dynamodb.UserRecord` to `user.User` and `user.User` to `user/dynamodb.UserRecord`. Handlers must never map directly to DynamoDB records.

## Package-by-feature consistency

The same feature package contains all layers for that feature:

```text
auth  → internal/auth/{domain.go,dto.go,mapper.go,service.go,handler.go,router.go}

user  → internal/user/{user.go,dto.go,mapper.go,service.go,handler.go,
        router.go,repository.go,dynamodb/*}

post  → internal/post/{post.go,dto.go,mapper.go,service.go,handler.go,
        router.go,repository.go,dynamodb/*}

ad    → internal/ad/{ad.go,dto.go,mapper.go,service.go,handler.go,
        router.go,repository.go,dynamodb/*}

feed  → internal/feed/{feed.go,dto.go,mapper.go,service.go,handler.go,
        router.go,repository.go,dynamodb/*}
```

The `auth` feature does not require a DynamoDB model or repository while authentication remains JWT-based and stateless. User credentials are persisted by the `user` feature, while JWT and bcrypt implementations belong to `platform`.

## Repository ports

Repository interfaces are defined in each feature's root `repository.go`, not inside its `dynamodb` implementation package. Their methods use feature entities or application contracts, never DynamoDB records.

```go
type UserRepository interface {
	GetByID(ctx context.Context, userID string) (User, error)
	GetByEmail(ctx context.Context, email string) (User, error)
	GetByUsername(ctx context.Context, username string) (User, error)
	Create(ctx context.Context, user User) error
	Update(ctx context.Context, user User) error
}
```

The DynamoDB repository implements this interface:

```go
type userRepository struct {
	tableName string
	client    *dynamodb.Client
}

func NewUserRepository(client *dynamodb.Client, tableName string) UserRepository {
	return &userRepository{
		client:    client,
		tableName: tableName,
	}
}
```

The service depends on `UserRepository`, never on `userRepository` or `*dynamodb.Client`.

Repositories should expose business-level results and domain errors. They should not expose DynamoDB `AttributeValue`, `ConditionalCheckFailedException`, or AWS SDK response types.

## Domain errors

Infrastructure errors must be translated before they leave the repository boundary. Domain errors are declared in the relevant feature root, for example `user.ErrNotFound`, `user.ErrEmailTaken`, or `post.ErrNotFound`.

Use sentinel errors or typed domain errors with `errors.Is` and `errors.As`. Cross-cutting errors such as unauthorized access, forbidden access, invalid input, and invalid cursors may be declared in a shared domain package if they are used by multiple contexts.

The error flow is:

```text
DynamoDB conditional failure
        ↓
repository maps it to ErrUsernameTaken
        ↓
service applies business context
        ↓
controller maps ErrUsernameTaken to HTTP 409
```

Unexpected infrastructure failures should be logged internally and mapped to a generic internal error. Raw database details must never be returned to clients.

## DynamoDB repository rules

The DynamoDB implementation is an adapter, not the application core.

It is responsible for:

- Table and index names
- Partition and sort key construction
- Query and update expressions
- Conditional writes
- DynamoDB transactions
- Pagination key conversion
- Mapping records to domain models
- Translating AWS SDK errors to domain errors

For username and email uniqueness, use a conditional uniqueness record and a DynamoDB transaction when creating a user. The service should request the operation through a uniqueness port; it should not know how the uniqueness record is stored.

## Lambda entry points and API groups

Each API group has its own Lambda entry point:

- `cmd/auth/main.go`
- `cmd/users/main.go`
- `cmd/posts/main.go`
- `cmd/ads/main.go`
- `cmd/feed/main.go`

Each entry point should:

1. Create an Fx application.
2. Register configuration, AWS clients, platform adapters, repositories, services, handlers, and routers with `fx.Provide`.
3. Resolve the API-group handler with `fx.Populate` or an Fx invoke.
4. Start the Fx application lifecycle.
5. Wrap the resolved router with `akrylysov/algnhsa`.
6. Start the Lambda handler.

Example composition root:

```go
func main() {
	var router http.Handler

	app := fx.New(
		fx.Provide(
			config.Load,
			dynamodbclient.New,
			dynamodbrepo.NewUserRepository,
			dynamodbrepo.NewUniquenessRepository,
			bcryptservice.New,
			jwtservice.New,
			clock.New,
			logger.New,
			service.NewAuthService,
			router.NewAuthRouter,
		),
		fx.Populate(&router),
	)

	ctx := context.Background()
	if err := app.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer app.Stop(ctx)

	lambda.Start(algnhsa.New(router))
}
```

The exact `algnhsa` adapter call should follow the version used in `go.mod`, but the dependency should remain confined to `cmd` or the Lambda adapter package.

## Chi routing

Chi is an HTTP adapter and belongs at the outer boundary.

```go
func NewUserRouter(service UserService) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(AuthMiddleware)

	r.Get("/api/v1/users/{userId}", getUser(service))
	r.Patch("/api/v1/users/me", updateMe(service))

	return r
}
```

Authentication middleware may parse and validate a JWT, but authorization decisions remain in the application service or a dedicated authorization policy. Middleware should place a small authenticated principal in the context rather than passing a JWT object through the domain.

## Authentication ports

JWT and bcrypt should be hidden behind interfaces.

```go
type TokenService interface {
	IssueAccessToken(ctx context.Context, principal Principal) (string, error)
	IssueRefreshToken(ctx context.Context, principal Principal) (string, error)
	ParseAccessToken(ctx context.Context, token string) (Principal, error)
	ParseRefreshToken(ctx context.Context, token string) (Principal, error)
}

type PasswordHasher interface {
	Hash(ctx context.Context, plaintext string) (string, error)
	Compare(ctx context.Context, hash string, plaintext string) error
}
```

The service layer depends on these interfaces. The bcrypt and JWT packages are replaceable infrastructure adapters.

Refresh tokens are opaque random values for the MVP. Only their SHA-256 hashes are stored in the refresh-session DynamoDB table. Rotation, logout, expiry, and session-family revocation are implemented through a refresh-session repository port. JWT signing keys are used only for access tokens and feed cursors.

## Dependency rules

The following import direction should be enforced:

```text
cmd/<group> → internal/<feature>/router
                    ↓
              handler → mapper → service
                                  ↓
                         feature entities/interfaces
                                  ↑
                     feature/dynamodb/repository
                                  ↓
                     feature/dynamodb/model
```

Rules:

1. Feature entities and domain errors import only the standard library and required shared domain types.
2. Feature services may depend on the feature's entities and interfaces.
3. Feature handlers may depend on the feature's services, DTOs, mappers, and HTTP packages.
4. Feature DynamoDB repositories may depend on the AWS SDK and the feature's repository interface.
5. DTOs must not be used as persistence records.
6. Feature `dynamodb/model.go` persistence records must not leave the feature's DynamoDB boundary.
7. `cmd` packages compose dependencies but do not implement use cases.
8. No package outside the DynamoDB adapter may import DynamoDB SDK types.
9. No service may inspect HTTP status codes or DynamoDB errors.
10. No handler may contain business decisions that belong to a service.

In particular, a feature service must not import its own `dto` definitions or its `dynamodb` subpackage. The handler-side mapper converts DTOs into domain commands, and the DynamoDB adapter maps persistence records into domain entities before returning from the repository.

## Testing strategy

The project uses three test levels:

- Unit tests with Ginkgo and Gomega
- Integration tests with Testcontainers
- End-to-end tests with Resty

### Coverage requirement

Automated tests must maintain at least **90% combined statement coverage for the complete Go project (`./...`)**. Coverage is a quality gate, not an optional reporting metric.

CI must:

1. Run unit, integration, and relevant E2E test suites.
2. Generate a combined Go coverage profile.
3. Report total coverage and coverage by feature/package.
4. Fail the build when combined project coverage is below 90%.
5. Publish the coverage report as a CI artifact.
6. Keep integration, E2E, Terraform, and non-Go files outside the Go statement profile naturally; do not exclude application Go packages merely to raise the percentage.

Coverage should include application services, domain rules, handlers, mappers, repository adapters, and error paths. Generated code and intentionally excluded bootstrap glue may be excluded only when documented in the coverage configuration; exclusions must not be used to hide untested business logic.

Example commands:

```bash
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

The CI pipeline should parse the combined project percentage from `go tool cover -func` and reject any result below 90%.

## Logging with Uber Zap

Use `go.uber.org/zap` for structured application logging. The logger should be created once in the composition root and injected through constructors into handlers, services, repositories, and background components that need it.

Prefer a typed logger interface in application code so business logic is not coupled to the concrete Zap implementation:

```go
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, err error, fields ...Field)
}
```

The platform adapter can wrap `*zap.Logger` or `*zap.SugaredLogger`, while feature packages depend on the small application logging interface.

Log important events needed to diagnose failures, latency, authorization problems, and feed behavior:

- Request start and completion
- Request ID, Lambda request ID, route, method, and status code
- Request duration and downstream dependency duration
- Authenticated user ID and required role, when safe and available
- Domain error code and operation name
- Repository operation and table/index name, without sensitive values
- Feed snapshot, cursor outcome, post count, ad count, and composition decision
- Authentication success/failure reason without credentials
- Conditional-write conflicts and uniqueness failures
- Unexpected dependency and infrastructure errors
- Startup configuration validation and graceful shutdown

Use structured fields rather than interpolated strings. Use `Info` for normal lifecycle events, `Warn` for expected but important conditions, and `Error` for failed operations or unexpected infrastructure problems. Include the original error as a wrapped field while returning a domain-safe error to the caller.

Never log passwords, JWTs, refresh tokens, authorization headers, raw request bodies, private media URLs, or sensitive personal data. Avoid logging full email addresses or other identifiers unless operationally necessary.

The logger must be flushed during graceful shutdown. Logging failures must not replace the original application error.

## Mandatory error handling

Errors must always be consumed and handled. Never silently discard an error with `_`.

Do not write either of these patterns when one returned value is an error:

```go
_, data := repository.GetByID(ctx, id)
data, _ := repository.GetByID(ctx, id)
```

They hide failures and can make the application continue with invalid or missing data.

Write explicit error handling instead:

```go
data, err := repository.GetByID(ctx, id)
if err != nil {
	logger.Error("failed to get user", err, FieldString("user_id", id))
	return user.User{}, err
}
return data, nil
```

If a returned value is intentionally unused, the error must still be assigned and handled explicitly:

```go
_, err := repository.Delete(ctx, id)
if err != nil {
	return err
}
```

The project should enable static analysis such as `errcheck`, `staticcheck`, and `govet` in CI. Error handling must be checked in handlers, services, mappers, repositories, constructors, shutdown hooks, logger flushes, and test setup/cleanup.

## CI/CD with GitHub Actions

Use GitHub Actions as the required CI/CD system. The workflow should run for pull requests and pushes to `main`.

The pipeline must:

- Check out the repository.
- Install the configured Go version.
- Run unit and integration tests as configured by the repository.
- Generate a coverage profile.
- Fail if combined Go project coverage is below 90%.
- Run static analysis and error checks.
- Build Lambda binaries for Linux ARM64.
- Package each API-group binary as a ZIP archive containing a `bootstrap` file.
- Upload ZIP files as workflow artifacts.
- Run Terraform formatting, validation, and infrastructure plan checks.
- Deploy application code only after successful checks and only from `main`.
- Use Terraform as the source of truth for AWS infrastructure.
- Use GitHub Actions with AWS OIDC to deploy Lambda ZIPs to Terraform-managed functions.

Terraform manages Lambda functions, configuration, API Gateway resources, IAM roles, DynamoDB tables, log groups, and related infrastructure. On pushes to `dev`, GitHub Actions builds all Lambda ZIPs and applies the complete Terraform plan, so infrastructure and Lambda code are deployed together.

Every public endpoint must be registered in all deployment layers: the feature
router, the Lambda composition root, and an `aws_apigatewayv2_route` in
`infra/dev/api_gateway.tf` targeting the correct Lambda integration. A route
that exists only in Chi or inside the Lambda is not reachable through AWS API
Gateway and is incomplete. Endpoint changes must include Terraform formatting,
validation, and plan review.

Because the project has multiple Lambda entry points, build one archive per command:

```text
cmd/auth/main.go  → build/auth-lambda.zip
cmd/users/main.go → build/users-lambda.zip
cmd/posts/main.go → build/posts-lambda.zip
cmd/ads/main.go   → build/ads-lambda.zip
cmd/feed/main.go  → build/feed-lambda.zip
```

### Required GitHub Actions workflow

The workflow should live at `.github/workflows/ci.yml`. This example builds the `auth` Lambda and runs Terraform; use a matrix or equivalent repeated jobs for `users`, `posts`, `ads`, and `feed`.

```yaml
name: CI

on:
  push:
    branches: ["main"]
  pull_request:

permissions:
  contents: read

env:
  GO_VERSION: "1.27.x"
  LAMBDA_ARCHIVE: build/auth-lambda.zip
  TF_IN_AUTOMATION: "true"

jobs:
  test-build:
    runs-on: ubuntu-latest
    steps:
      - name: Check out source
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true

      - name: Run tests and collect coverage
        run: |
          go test ./... -coverprofile=coverage.out
          go tool cover -func=coverage.out | tee coverage.txt

      - name: Enforce 90 percent coverage
        run: |
          coverage=$(tail -n 1 coverage.txt | awk '{print substr($3, 1, length($3)-1)}')
          awk -v coverage="$coverage" 'BEGIN { if (coverage < 90.0) exit 1 }'

      - name: Run static analysis
        run: |
          go vet ./...
          go run honnef.co/go/tools/cmd/staticcheck@latest ./...
          go run github.com/kisielk/errcheck@latest ./...

      - name: Build Lambda binary
        env:
          GOOS: linux
          GOARCH: arm64
          CGO_ENABLED: 0
        run: |
          mkdir -p build/auth
          go build -trimpath -ldflags='-s -w' -o build/auth/bootstrap ./cmd/auth

      - name: Package Lambda
        run: (cd build/auth && zip -j ../auth-lambda.zip bootstrap)

      - name: Upload Lambda package
        uses: actions/upload-artifact@v4
        with:
          name: auth-lambda-package
          path: |
            build/auth-lambda.zip
            coverage.out
          if-no-files-found: error

      - name: Set up Terraform
        uses: hashicorp/setup-terraform@v3

      - name: Terraform format check
        working-directory: infra/dev
        run: terraform fmt -check -recursive

      - name: Terraform init
        working-directory: infra/dev
        run: terraform init -input=false

      - name: Terraform validate
        working-directory: infra/dev
        run: terraform validate

      - name: Upload Terraform inputs
        uses: actions/upload-artifact@v4
        with:
          name: terraform-inputs
          path: infra/dev
          if-no-files-found: error

  plan:
    if: github.event_name == 'pull_request'
    needs: test-build
    runs-on: ubuntu-latest
    permissions:
      contents: read
      id-token: write
    steps:
      - name: Check out source
        uses: actions/checkout@v4

      - name: Download Lambda package
        uses: actions/download-artifact@v4
        with:
          name: auth-lambda-package

      - name: Configure AWS credentials for Terraform plan
        uses: aws-actions/configure-aws-credentials@v4
        with:
          aws-region: ${{ secrets.AWS_REGION }}
          role-to-assume: ${{ secrets.AWS_ROLE_ARN }}

      - name: Set up Terraform
        uses: hashicorp/setup-terraform@v3

      - name: Terraform init
        working-directory: infra/dev
        run: terraform init -input=false

      - name: Terraform plan
        working-directory: infra/dev
        run: terraform plan -input=false -out=tfplan

      - name: Upload Terraform plan
        uses: actions/upload-artifact@v4
        with:
          name: terraform-plan
          path: infra/dev/tfplan
          if-no-files-found: error

  deploy:
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    needs: test-build
    runs-on: ubuntu-latest
    permissions:
      contents: read
      id-token: write
    steps:
      - name: Check out source
        uses: actions/checkout@v4

      - name: Download Lambda package
        uses: actions/download-artifact@v4
        with:
          name: auth-lambda-package

      - name: Configure AWS credentials for Lambda deployment
        uses: aws-actions/configure-aws-credentials@v4
        with:
          aws-region: ${{ secrets.AWS_REGION }}
          role-to-assume: ${{ secrets.AWS_ROLE_ARN }}

      - name: Deploy Lambda code
        run: >-
          aws lambda update-function-code
          --function-name "social-media-dev-auth"
          --zip-file fileb://${{ env.LAMBDA_ARCHIVE }}
```

### Multi-Lambda workflow rules

For the complete application, build one ZIP archive per command and pass all archives to the complete Terraform plan. Terraform creates, configures, and updates each corresponding Lambda function together with its infrastructure.

Each Lambda function must be configured with:

- `arm64` architecture
- A handler value of `bootstrap`
- The same Go runtime-compatible custom runtime configuration
- Its own function name and environment configuration

The Terraform deployment role should have only the permissions required to manage the intended resources. AWS credentials must not be stored as long-lived access keys in GitHub secrets; use the OIDC role-assumption flow shown above.

Coverage, test, build, and deployment failures must stop the pipeline. Deployment must never run when tests, static analysis, error checks, coverage enforcement, compilation, or packaging fail.

## Terraform and GitOps rules

Terraform configuration is the desired state for the application infrastructure. Git is the source of truth, and infrastructure changes must be made through pull requests rather than manual console changes or ad-hoc AWS CLI commands.

Recommended Terraform layout:

```text
infra/
├── dev/
│   └── *.tf
└── local/
├── backend.tf
├── providers.tf
├── variables.tf
├── locals.tf
├── lambda.tf
├── api_gateway.tf
├── dynamodb.tf
├── iam.tf
├── cloudwatch.tf
├── outputs.tf
└── environments/
    ├── dev.tfvars
    └── prod.tfvars
```

Use `/infra/dev` for the initial development Terraform stack, not `/_infra`. Terraform files use the `.tf` extension and are not compiled or imported by the Go compiler. Keeping infrastructure under the normal `infra` directory is clearer for Terraform, GitHub Actions, IDEs, and operators. Each environment should contain Terraform files only; if Go helper code is ever needed, keep it in a separate Go package outside the Terraform configuration directory.

Terraform should manage:

- Lambda functions and ARM64 configuration
- Lambda IAM execution roles
- API Gateway routes and integrations
- DynamoDB tables and indexes
- CloudWatch log groups and retention
- SSM Parameter Store parameter names, types, and access policies
- Environment variables and configuration references
- Initial Lambda bootstrap artifacts required to create functions

Terraform creates each Lambda with an initial bootstrap archive. Subsequent application-code updates are owned by GitHub Actions, so Terraform must ignore code-artifact changes:

```hcl
resource "aws_lambda_function" "auth" {
  function_name    = var.auth_function_name
  filename         = var.initial_bootstrap_archive
  source_code_hash = filebase64sha256(var.initial_bootstrap_archive)
  role             = aws_iam_role.lambda.arn
  handler          = "bootstrap"
  runtime          = "provided.al2023"
  architectures    = ["arm64"]

  lifecycle {
    ignore_changes = [filename, source_code_hash]
  }
}
```

Terraform state must use a protected remote backend with encryption, versioning, and state locking. State files must never be committed to Git or uploaded as ordinary build artifacts. Production applies should use a protected GitHub environment with required reviewers.

Secret values must not be committed to Terraform files or ordinary GitHub Actions variables. If Terraform creates `SecureString` parameters, protect the Terraform state because sensitive values can be represented in state. Prefer provisioning secret values through a protected secret-management process while keeping parameter names, IAM access, and non-secret configuration in Terraform.

The GitOps workflow is:

```text
Pull request
    ↓
tests + coverage + static analysis + Terraform fmt/validate
    ↓
Terraform plan reviewed in the pull request
    ↓
merge to main
    ↓
build ARM64 Lambda archives
    ↓
AWS OIDC-authenticated Lambda code deployment
    ↓
Terraform-managed infrastructure is applied together with the Lambda archives
```

Terraform plans are previews and must be reviewed before infrastructure changes are applied. On `dev` pushes, the deployment job applies the complete Terraform plan, including infrastructure and Lambda code artifacts. No separate Lambda-only deployment path is used.

### Unit tests: Ginkgo and Gomega

Unit tests should run without AWS, Docker, Lambda, HTTP servers, or DynamoDB. Use Ginkgo for test organization and Gomega for assertions.

Recommended unit-test targets:

- Feature entities and domain rules
- Application services with fake repositories
- Feed composition and cursor behavior
- Request DTO validation
- Mappers
- Domain error mapping

Example layout:

```text
internal/feed/feed_suite_test.go
internal/feed/feed_test.go
internal/user/service_test.go
internal/user/mapper_test.go
```

Use fakes or in-memory adapters for service tests. Do not start containers in unit tests.

### Domain and feed tests

Test feed rules and invariants without AWS, HTTP, or Lambda:

- 20 posts plus 1 ad
- Fewer than 20 final-page posts without an additional ad
- No eligible ads
- Empty feed
- Invalid cursor
- Stable ordering by creation time
- Same-timestamp tie-breaking

### Service tests

Use fake repositories and fake platform ports to test:

- Registration
- Duplicate email and username handling
- Login and token issuance
- Ownership checks
- Admin authorization
- Post and ad lifecycle rules
- Cursor handling

### Handler tests

Use `httptest` with mocked services to test:

- JSON decoding, body-size limits, and validation
- Unknown-field rejection
- HTTP status codes
- Error response format
- Authentication context handling
- DTO mapping

### Integration tests: Testcontainers

Run repository and adapter tests using Testcontainers. The preferred container is DynamoDB Local; additional containers may be added when the application introduces services such as Redis or LocalStack.

Integration tests should verify:

- Key construction
- Index queries
- Conditional uniqueness writes
- Transactions
- DynamoDB pagination behavior
- Error translation

Integration tests should use isolated tables, deterministic test data, and explicit container lifecycle management. They should not depend on developer credentials or an already-running local database.

The repository integration suite is the single `tests/integration` Go package.
Start exactly one DynamoDB Local Testcontainers instance in its Ginkgo
`BeforeSuite`, create the required client and tables once, and terminate the
container once in `AfterSuite`. All integration specs reuse that endpoint.
Tests remain independent by using isolated table names, deterministic keys,
and cleanup/reset operations between specs. Do not call `GenericContainer`,
`Run`, or `Terminate` from an individual `It` block. The support package may
test lifecycle error handling, but must not start a second normal integration
database container.

Do not split repository integration specs into multiple Go packages or run
them in parallel: one `just integration` invocation must own one DynamoDB
Local container. Do not enable persistent Testcontainers reuse in CI;
suite-scoped lifecycle plus explicit table isolation prevents stale state.

Fixtures and support code have separate responsibilities:

- `tests/fixtures/` contains deterministic users, posts, advertisements,
  tokens, request bodies, timestamps, and fixture builders.
- `tests/integration/support/` contains Testcontainers lifecycle, DynamoDB
  clients, table creation/deletion, reset helpers, and shared assertions.
- `tests/e2e/support/` contains Resty clients and deployed-environment helpers.

Fixtures must not start containers or access AWS. Support helpers must not hide
test data or silently weaken assertions.

Example layout:

```text
tests/
├── fixtures/
│   ├── auth.go
│   ├── posts.go
│   └── advertisements.go
├── integration/
│   ├── auth/
│   │   ├── auth_suite_test.go
│   │   ├── register_test.go
│   │   ├── login_test.go
│   │   └── refresh_test.go
│   └── support/
│       ├── containers.go
│       └── dynamodb.go
└── e2e/
    └── support/
        └── resty.go
```

### End-to-end tests: Resty and SAM Local

End-to-end tests should exercise the deployed or locally assembled HTTP API through its public routes. Use Resty as the HTTP client.

Run local E2E tests with `just e2e`. SAM Local serves the public routes and
launches Lambda containers, while one suite-scoped DynamoDB Local
Testcontainer supplies isolated tables. E2E tests must not use AWS or the
developer-managed Compose database.

E2E tests should verify complete user flows:

- Register and log in
- Refresh an access token
- Create a post
- Create an advertisement as an admin
- Retrieve the mixed feed
- Verify the 20-post-plus-1-ad and partial-feed rules
- Verify authorization failures and validation responses

E2E tests are flow tests, not endpoint smoke tests. A new public endpoint must
extend the journey it participates in or add a multi-request journey. Keep
separate flow files where useful, but share one SAM Local process and one
suite-scoped DynamoDB Local Testcontainer for the complete E2E run.

The E2E environment should provide its own configuration, tables, secrets, and test users. Tests must not run against production data.

Example layout:

```text
tests/e2e/e2e_test.go
tests/e2e/support/environment.go
```

### Lambda smoke tests

Test each command package to confirm that its router and Lambda adapter can be constructed with test configuration.

## Composition and shared infrastructure

Each API group may have a separate Lambda, but shared construction should not be duplicated. Use small composition helpers under `internal/shared` or `cmd` while keeping API-specific service composition explicit.

For example:

```go
type Dependencies struct {
	Users      user.UserRepository
	Posts      post.PostRepository
	Ads        ad.AdRepository
	Tokens     TokenService
	Passwords  PasswordHasher
	Clock      Clock
}
```

Avoid creating global clients or global mutable services. AWS SDK clients can be initialized once per warm Lambda container and reused safely, but their ownership should remain inside the composition root.

## Observability and reliability

The outer adapter layers should add:

- Request IDs
- Lambda request IDs
- Route names
- Authenticated user IDs where available
- HTTP status codes
- Request duration
- Structured error codes

Never log passwords, JWTs, refresh tokens, raw authorization headers, or private media URLs.

Services should accept a clock interface so timestamp-dependent behavior and feed tests are deterministic.

Repository calls should receive the request context so cancellation, deadlines, and tracing propagate through Lambda, services, and DynamoDB.

## Architectural goal

The project should be able to replace:

- DynamoDB with PostgreSQL or another database
- Lambda with a long-running HTTP server
- Chi with another HTTP router
- JWT implementation with an identity provider
- bcrypt with another password hashing adapter

without changing the core domain rules and application services.
