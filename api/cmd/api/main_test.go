package main

import "testing"

func TestSecureCookieForEnv(t *testing.T) {
	cases := []struct {
		appEnv string
		want   bool
	}{
		{appEnv: "dev", want: false},
		{appEnv: "test", want: false},
		{appEnv: "prod", want: true},
		{appEnv: "", want: true},
	}
	for _, c := range cases {
		if got := secureCookieForEnv(c.appEnv); got != c.want {
			t.Fatalf("secureCookieForEnv(%q)=%v want %v", c.appEnv, got, c.want)
		}
	}
}
