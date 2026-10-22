package auth

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ValidateBoundaries", func() {
	It("runs the test", func() {
		cases := []struct {
			name string
			c    RegisterCommand
			ok   bool
		}{{"valid", RegisterCommand{"abc", "a@b.com", "123456789012", "A"}, true}, {"username uppercase", RegisterCommand{"Abc", "a@b.com", "123456789012", "A"}, false}, {"password bytes", RegisterCommand{"abc", "a@b.com", "12345678901", "A"}, false}, {"display unicode", RegisterCommand{"abc", "a@b.com", "123456789012", "😀"}, true}}
		for _, tc := range cases {
			_, e := Validate(tc.c)
			if (len(e) == 0) != tc.ok {
				Expect(false).To(BeTrue(), fmt.Sprintf("%s errors=%v", tc.name, e))
			}
		}

	})
})

var _ = Describe("ValidateRejectsContractViolations", func() {
	It("runs the test", func() {
		base := RegisterCommand{"abc", "a@b.com", "123456789012", "A"}
		cases := []struct {
			name   string
			mutate func(*RegisterCommand)
		}{
			{"username too long", func(c *RegisterCommand) { c.Username = "abcdefghijklmnopqrstuvwxyz12345" }},
			{"username invalid character", func(c *RegisterCommand) { c.Username = "abc-" }},
			{"email too long", func(c *RegisterCommand) { c.Email = "a@" + string(make([]byte, 253)) }},
			{"email with display name", func(c *RegisterCommand) { c.Email = "Alice <a@b.com>" }},
			{"password too long in bytes", func(c *RegisterCommand) { c.Password = string(make([]byte, 73)) }},
			{"invalid display name", func(c *RegisterCommand) { c.DisplayName = string([]byte{0xff}) }},
			{"display name too long", func(c *RegisterCommand) { c.DisplayName = string(make([]byte, 81)) }},
		}
		for _, tc := range cases {
			{
				candidate := base
				tc.mutate(&candidate)
				if _, errs := Validate(candidate); len(errs) == 0 {
					Expect(false).To(BeTrue(), fmt.Sprint("accepted invalid registration"))
				}
			}
		}

	})
})
