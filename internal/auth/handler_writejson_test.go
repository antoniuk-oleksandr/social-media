package auth

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http/httptest"
	"strings"
)

var _ = Describe("WriteJSONFallsBackWhenValueCannotBeMarshaled", func() {
	It("runs the test", func() {
		w := httptest.NewRecorder()
		writeJSON(w, 200, make(chan int), "req-fallback")

		if w.Code != 500 {
			Expect(false).To(BeTrue(), fmt.Sprintf("status = %d, want 500", w.Code))
		}
		if got := w.Header().Get("Content-Type"); got != "application/json" {
			Expect(false).To(BeTrue(), fmt.Sprintf("content type = %q, want application/json", got))
		}
		if !strings.Contains(w.Body.String(), `"code":"INTERNAL_ERROR"`) || !strings.Contains(w.Body.String(), `"requestId":"req-fallback"`) {
			Expect(false).To(BeTrue(), fmt.Sprintf("fallback body = %s", w.Body.String()))
		}

	})
})
