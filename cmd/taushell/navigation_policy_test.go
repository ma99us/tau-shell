package main

import "testing"

func TestIsBundledAppNavigation(t *testing.T) {
	tests := []struct {
		target string
		want   bool
	}{
		{target: "http://wails.localhost/", want: true},
		{target: "HTTP://WAILS.LOCALHOST/app/index.html?tab=1#terminal", want: true},
		{target: "http://wails.localhost:80/", want: true},
		{target: "https://wails.localhost/", want: false},
		{target: "http://wails.localhost:8080/", want: false},
		{target: "http://wails.localhost.example.com/", want: false},
		{target: "http://wails.localhost@evil.example/", want: false},
		{target: "http://wails.localhost\\@evil.example/", want: false},
		{target: "http://wails.localhost./", want: false},
		{target: "http://evil.example/", want: false},
		{target: "javascript:alert(1)", want: false},
		{target: "", want: false},
	}

	for _, test := range tests {
		t.Run(test.target, func(t *testing.T) {
			if got := isBundledAppNavigation(test.target); got != test.want {
				t.Errorf("isBundledAppNavigation(%q) = %t, want %t", test.target, got, test.want)
			}
		})
	}
}
