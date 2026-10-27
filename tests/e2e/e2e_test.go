//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	e2esupport "github.com/antoniuk-oleksandr/social-media/tests/e2e/support"
	"github.com/antoniuk-oleksandr/social-media/tests/fixtures"
	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type authResponse struct {
	User struct {
		ID string `json:"id"`
	} `json:"user"`
	AccessToken      string `json:"accessToken"`
	RefreshToken     string `json:"refreshToken"`
	RefreshExpiresIn int64  `json:"refreshExpiresIn"`
}

type postResponse struct {
	ID string `json:"id"`
}

const e2eAdminID = fixtures.AdminID

var (
	e2eEnvironment *e2esupport.Environment
	e2eClient      *resty.Client
	samCommand     *exec.Cmd
	samEnvFile     string
	samContainers  map[string]struct{}
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "local SAM E2E suite")
}

var _ = BeforeSuite(func() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var err error
	e2eEnvironment, err = e2esupport.Start(ctx)
	Expect(err).NotTo(HaveOccurred(), "E2E DynamoDB environment must start")
	Expect(e2eEnvironment.SeedAdmin(ctx, e2eAdminID, fixtures.AdminCredentials)).To(Succeed())

	port := reservePort()
	samEnvFile, err = writeSAMEnvironment(e2eEnvironment)
	Expect(err).NotTo(HaveOccurred())
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	Expect(err).NotTo(HaveOccurred())
	samContainers = dockerContainerIDs()

	samCommand = exec.Command(
		"sam", "local", "start-api",
		"--template-file", filepath.Join(repositoryRoot, "infra", "local", "template.yaml"),
		"--env-vars", samEnvFile,
		"--docker-network", e2eEnvironment.Network.Name,
		"--add-host", "host.docker.internal:host-gateway",
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"--warm-containers", "LAZY",
		"--skip-pull-image",
	)
	samCommand.Stdout = os.Stdout
	samCommand.Stderr = os.Stderr
	Expect(samCommand.Start()).To(Succeed())
	e2eClient = resty.New().SetBaseURL(fmt.Sprintf("http://127.0.0.1:%d", port)).SetHeader("Content-Type", "application/json")
	Expect(waitForSAM(e2eClient)).To(Succeed(), "SAM Local API must start")
})

var _ = AfterSuite(func() {
	if samCommand != nil && samCommand.Process != nil {
		_ = samCommand.Process.Signal(os.Interrupt)
		waited := make(chan struct{})
		go func() {
			_, _ = samCommand.Process.Wait()
			close(waited)
		}()
		select {
		case <-waited:
		case <-time.After(5 * time.Second):
			_ = samCommand.Process.Kill()
			<-waited
		}
	}
	if samEnvFile != "" {
		_ = os.Remove(samEnvFile)
	}
	removeCreatedContainers(samContainers)
	if e2eEnvironment != nil {
		Expect(e2eEnvironment.Close(context.Background())).To(Succeed())
	}
})

func request(method, path string, body any, token string, result any) *resty.Response {
	req := e2eClient.R()
	if token != "" {
		req.SetHeader("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.SetBody(body)
	}
	if result != nil {
		req.SetResult(result)
	}
	response, err := req.Execute(method, path)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return response
}

func reservePort() int {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitForSAM(client *resty.Client) error {
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		// Probe an unmapped route so readiness does not start a Lambda container
		// or produce application-level dependency errors.
		response, err := client.R().Get("/__e2e_ready__")
		if err == nil && response.StatusCode() > 0 {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("SAM Local API did not start before timeout")
}

func writeSAMEnvironment(environment *e2esupport.Environment) (string, error) {
	common := map[string]string{
		"AWS_ACCESS_KEY_ID": "dummy", "AWS_SECRET_ACCESS_KEY": "dummy", "AWS_REGION": "us-east-1",
		"APP_ENV": "local", "JWT_KID": "local", "JWT_ACCESS_SECRET": "local-development-secret",
		"JWT_ISSUER": "social-media-api", "JWT_AUDIENCE": "social-media-api",
		"DYNAMODB_ENDPOINT": environment.DynamoDBEndpoint, "USERS_TABLE": environment.Suite.UsersTable,
		"UNIQUES_TABLE": environment.Suite.UniquesTable, "REFRESH_TABLE": environment.Suite.RefreshTable,
		"POSTS_TABLE": environment.PostsTable, "ADS_TABLE": environment.AdsTable,
		"ADS_CURSOR_KEY": "e2e-local-ads-cursor-key", "ADS_CURSOR_KEY_ID": "e2e",
	}
	values := map[string]map[string]string{"Parameters": common}
	for _, function := range []string{"AuthFunction", "UsersFunction", "PostsFunction", "AdsFunction", "FeedFunction"} {
		values[function] = common
	}
	file, err := os.CreateTemp("", "social-media-e2e-env-*.json")
	if err != nil {
		return "", err
	}
	defer file.Close()
	if err := json.NewEncoder(file).Encode(values); err != nil {
		_ = os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

func dockerContainerIDs() map[string]struct{} {
	result, err := exec.Command("docker", "ps", "-aq").Output()
	if err != nil {
		return nil
	}
	ids := make(map[string]struct{})
	for _, id := range splitLines(string(result)) {
		ids[id] = struct{}{}
	}
	return ids
}

func removeCreatedContainers(before map[string]struct{}) {
	after := dockerContainerIDs()
	created := make([]string, 0)
	for id := range after {
		if _, exists := before[id]; !exists {
			created = append(created, id)
		}
	}
	if len(created) > 0 {
		_ = exec.Command("docker", append([]string{"rm", "--force"}, created...)...).Run()
	}
}

func splitLines(value string) []string {
	lines := make([]string, 0)
	for _, line := range strings.Split(value, "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
