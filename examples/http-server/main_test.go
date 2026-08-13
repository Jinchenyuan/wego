package main

import "testing"

func TestHTTPPort(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    int
		wantErr bool
	}{
		{name: "default", want: 8080},
		{name: "configured", value: "9090", want: 9090},
		{name: "not a number", value: "http", wantErr: true},
		{name: "zero", value: "0", wantErr: true},
		{name: "above maximum", value: "65536", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("WEGO_HTTP_PORT", tt.value)
			got, err := httpPort()
			if (err != nil) != tt.wantErr {
				t.Fatalf("httpPort() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("httpPort() = %d, want %d", got, tt.want)
			}
		})
	}
}
