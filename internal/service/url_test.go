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
	for _, valid := range []string{"18", "best", "h264-720p", "format_1"} {
		if err := ValidateFormatID(valid, false); err != nil {
			t.Errorf("expected %q to be valid: %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "137+140", "137+140+251", "18/best", "--help", "id value", "137+"} {
		if err := ValidateFormatID(invalid, false); err == nil {
			t.Errorf("expected %q to be invalid", invalid)
		}
	}
	if err := ValidateFormatID("137+140", true); err != nil {
		t.Errorf("expected DASH format to be valid with provider: %v", err)
	}
	if err := ValidateFormatID("137+140+251", true); err == nil {
		t.Error("expected three-part DASH format to be invalid")
	}
}
