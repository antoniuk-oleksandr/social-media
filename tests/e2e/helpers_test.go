//go:build e2e

package e2e

import (
	"net/http"
	"strconv"
	"time"

	"github.com/antoniuk-oleksandr/social-media/tests/fixtures"
	. "github.com/onsi/gomega"
)

type e2eAccount struct {
	authResponse
	Login    string
	Password string
}

func registerE2EUser() e2eAccount {
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	credentials := fixtures.UserCredentials(suffix)
	result := e2eAccount{Login: credentials.Email, Password: credentials.Password}
	response := request(http.MethodPost, "/api/v1/auth/register", map[string]string{
		"username": credentials.Username, "email": credentials.Email,
		"password": credentials.Password, "displayName": credentials.DisplayName,
	}, "", &result)
	Expect(response.StatusCode()).To(Equal(http.StatusCreated), response.String())
	Expect(result.User.ID).NotTo(BeEmpty())
	Expect(result.AccessToken).NotTo(BeEmpty())
	Expect(result.RefreshToken).NotTo(BeEmpty())
	return result
}

func loginE2E(login, password string) authResponse {
	result := authResponse{}
	response := request(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"login": login, "password": password,
	}, "", &result)
	Expect(response.StatusCode()).To(Equal(http.StatusOK), response.String())
	Expect(result.AccessToken).NotTo(BeEmpty())
	Expect(result.RefreshToken).NotTo(BeEmpty())
	return result
}

func loginE2EAdmin() authResponse {
	return loginE2E(fixtures.AdminCredentials.Email, fixtures.AdminCredentials.Password)
}
