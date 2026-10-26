package fx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	platformconfig "github.com/antoniuk-oleksandr/social-media/internal/platform/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	. "github.com/onsi/gomega"
)

func TestNewConfiguredAdsConfig(t *testing.T) {
	g := NewWithT(t)
	for _, key := range []string{"APP_ENV", "ADS_CURSOR_KEY", "ADS_CURSOR_KEY_PARAMETER", "ADS_TABLE", "ADS_CURSOR_KEY_ID"} {
		t.Setenv(key, "")
	}
	t.Setenv("APP_ENV", "local")
	t.Setenv("ADS_CURSOR_KEY", "local-cursor-key")
	t.Setenv("ADS_TABLE", "ads-test")
	t.Setenv("ADS_CURSOR_KEY_ID", "local-kid")

	cfg, err := NewConfiguredAdsConfig(aws.Config{})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg).To(Equal(platformconfig.AdsConfig{AdsTable: "ads-test", CursorKey: "local-cursor-key", CursorKeyID: "local-kid", CursorVersion: 1}))
}

func TestNewConfiguredAdsConfigRequiresProductionParameter(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("ADS_CURSOR_KEY", "")
	t.Setenv("ADS_CURSOR_KEY_PARAMETER", "")

	_, err := NewConfiguredAdsConfig(aws.Config{})
	g.Expect(err).To(MatchError("ADS_CURSOR_KEY_PARAMETER is required"))
}

func TestNewConfiguredAdsConfigLoadsSSMKey(t *testing.T) {
	g := NewWithT(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.Expect(r.Header.Get("X-Amz-Target")).To(Equal("AmazonSSM.GetParameter"))
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, err := w.Write([]byte(`{"Parameter":{"Name":"/social-media/feed","Type":"SecureString","Value":"ssm-cursor-key","Version":1}}`))
		g.Expect(err).NotTo(HaveOccurred())
	}))
	defer server.Close()
	t.Setenv("APP_ENV", "production")
	t.Setenv("ADS_CURSOR_KEY", "")
	t.Setenv("ADS_CURSOR_KEY_PARAMETER", "/social-media/ads")
	t.Setenv("ADS_TABLE", "ads-production")

	cfg := aws.Config{Region: "us-east-1", BaseEndpoint: aws.String(server.URL), Credentials: credentials.NewStaticCredentialsProvider("test", "test", "")}
	got, err := NewConfiguredAdsConfig(cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.CursorKey).To(Equal("ssm-cursor-key"))
	g.Expect(got.AdsTable).To(Equal("ads-production"))
}
