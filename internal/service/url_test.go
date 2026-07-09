package service

import "testing"

func TestValidateMediaURL(t *testing.T) {
	tests := []struct {
		name  string
		url   string
		valid bool
	}{
		{"YouTube", "https://www.youtube.com/watch?v=abc", true},
		{"YouTube short URL", "https://youtu.be/abc", true},
		{"X", "https://x.com/user/status/123", true},
		{"Twitter", "https://twitter.com/user/status/123", true},
		{"mobile Twitter", "https://mobile.twitter.com/user/status/123", true},
		{"tiktok", "https://www.tiktok.com/@user/video/123", true},
		{"subdomain confusion", "https://x.com.example.org/user/status/123", false},
		{"suffix confusion", "https://evilx.com/user/status/123", false},
		{"credentials", "https://user:pass@x.com/user/status/123", false},
		{"IP address", "https://127.0.0.1/video", false},
		{"insecure scheme", "http://x.com/user/status/123", false},
		{"YouTube subdomain confusion", "https://youtube.com.example.org/watch?v=abc", false},
		{"unsupported", "https://example.com/video", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateMediaURL(test.url)
			if test.valid && err != nil {
				t.Fatalf("expected valid URL, got %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("expected URL validation error")
			}
		})
	}
}

func TestValidateFormatID(t *testing.T) {
	for _, valid := range []string{"18", "best", "h264-720p", "format_1", "137+140"} {
		if err := ValidateFormatID(valid); err != nil {
			t.Errorf("expected %q to be valid: %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "137+140+251", "18/best", "--help", "id value", "137+"} {
		if err := ValidateFormatID(invalid); err == nil {
			t.Errorf("expected %q to be invalid", invalid)
		}
	}
}
