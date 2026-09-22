package main

import "testing"

func TestResolveListenPortPrecedence(t *testing.T) {
	configured := 8080
	tests := []struct {
		name       string
		cliPort    int
		cliPortSet bool
		configPort *int
		want       int
	}{
		{name: "CLI overrides config", cliPort: 9090, cliPortSet: true, configPort: &configured, want: 9090},
		{name: "config overrides default", configPort: &configured, want: 8080},
		{name: "default", want: 6480},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveListenPort(test.cliPort, test.cliPortSet, test.configPort); got != test.want {
				t.Fatalf("resolveListenPort() = %d, want %d", got, test.want)
			}
		})
	}
}
