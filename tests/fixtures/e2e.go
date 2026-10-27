package fixtures

import "time"

type Credentials struct {
	Username    string
	Email       string
	Password    string
	DisplayName string
}

func UserCredentials(seed string) Credentials {
	return Credentials{Username: "e2e_" + seed, Email: "e2e-" + seed + "@example.com", Password: "correct-password", DisplayName: "E2E User"}
}

var AdminCredentials = Credentials{Username: "e2e_admin", Email: "e2e-admin@example.com", Password: "admin-password", DisplayName: "E2E Administrator"}

const AdminID = "0192f8a0-7b00-7000-8000-000000000901"

type AdvertisementRequest struct {
	Advertiser string `json:"advertiser"`
	Content    string `json:"content"`
	TargetURL  string `json:"targetUrl"`
	StartsAt   string `json:"startsAt"`
	EndsAt     string `json:"endsAt"`
	Status     string `json:"status"`
}

func ActiveAdvertisement(content string, now time.Time) AdvertisementRequest {
	return AdvertisementRequest{
		Advertiser: "E2E Advertiser", Content: content, TargetURL: "https://example.com/e2e",
		StartsAt: now.UTC().Add(-time.Hour).Format(time.RFC3339), EndsAt: now.UTC().Add(time.Hour).Format(time.RFC3339), Status: "active",
	}
}
