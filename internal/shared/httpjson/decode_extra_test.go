package httpjson

import (
	"errors"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http"
	"net/http/httptest"
	"strings"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (failingReader) Close() error             { return nil }

var _ = Describe("DecodeRejectsOversizedBody", func() {
	It("runs the test", func() {
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"`+strings.Repeat("x", 64<<10)+`"}`))
		if err := Decode(r, &struct{}{}); err == nil {
			Expect(false).To(BeTrue(), fmt.Sprint("accepted oversized body"))
		}

	})
})

var _ = Describe("DecodeScansNestedObjectsAndArrays", func() {
	It("runs the test", func() {
		for _, body := range []string{`{"nested":{"value":1}}`, `{"nested":[{"value":1},2]}`} {
			r := httptest.NewRequest("POST", "/", strings.NewReader(body))
			var dst map[string]any
			if err := Decode(r, &dst); err != nil {
				Expect(false).To(BeTrue(), fmt.Sprintf("body %s: %v", body, err))
			}
		}

	})
})

var _ = Describe("DecodePropagatesBodyReadError", func() {
	It("runs the test", func() {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"))
		r.Body = failingReader{}
		if err := Decode(r, &map[string]any{}); err == nil || !strings.Contains(err.Error(), "read failed") {
			Expect(false).To(BeTrue(), fmt.Sprintf("Decode() error = %v, want read failure", err))
		}

	})
})

var _ = Describe("ScanValueRejectsMalformedNestedValue", func() {
	It("runs the test", func() {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"nested":`))
		if err := Decode(r, &map[string]any{}); err == nil {
			Expect(false).To(BeTrue(), fmt.Sprint("Decode() accepted malformed nested value"))
		}

	})
})
