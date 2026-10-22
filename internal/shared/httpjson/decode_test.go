package httpjson

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http/httptest"
	"strings"
)

type payload struct {
	Name   string         `json:"name"`
	Nested map[string]any `json:"nested"`
	Values []int          `json:"values"`
}

func req(body string) *httptest.ResponseRecorder { return httptest.NewRecorder() }

var _ = Describe("DecodeAcceptsObjectAndNestedValues", func() {
	It("runs the test", func() {
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"x","nested":{"a":[1,true]},"values":[1,2]}`))
		var p payload
		if e := Decode(r, &p); e != nil || p.Name != "x" {
			Expect(false).To(BeTrue(), fmt.Sprintf("%v %+v", e, p))
		}

	})
})
var _ = Describe("DecodeRejectsInvalidBodies", func() {
	It("runs the test", func() {
		for _, body := range []string{"", "[]", `{"a":1,"a":2}`, `{"a": [1,]}`, `{"a":1} {}`} {
			r := httptest.NewRequest("POST", "/", strings.NewReader(body))
			if e := Decode(r, &struct{}{}); e == nil {
				Expect(false).To(BeTrue(), fmt.Sprintf("accepted %q", body))
			}
		}

	})
})
var _ = Describe("DecodeRejectsUnknownField", func() {
	It("runs the test", func() {
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"unknown":1}`))
		if e := Decode(r, &struct {
			Name string `json:"name"`
		}{}); e == nil {
			Expect(false).To(BeTrue(), fmt.Sprint("accepted unknown"))
		}

	})
})

var _ = Describe("DecodeRejectsScalarRoot", func() {
	It("runs the test", func() {
		r := httptest.NewRequest("POST", "/", strings.NewReader(`1`))
		if err := Decode(r, &payload{}); err == nil {
			Expect(false).To(BeTrue(), fmt.Sprint("accepted scalar root"))
		}

	})
})
