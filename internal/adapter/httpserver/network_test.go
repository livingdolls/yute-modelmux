package httpserver

import "testing"

func TestIsLoopbackBindHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"127.0.0.1", true},
		{"127.42.0.1", true},
		{"::1", true},
		{"localhost", true},
		{"0.0.0.0", false},
		{"::", false},
		{"192.168.1.20", false},
		{"example.invalid", false},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := isLoopbackBindHost(tt.host); got != tt.want {
				t.Fatalf("isLoopbackBindHost(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}
