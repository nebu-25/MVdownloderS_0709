package service

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

var ErrInvalidURL = errors.New("invalid or unsupported URL")

var allowedHosts = []string{
	"youtube.com",
	"youtu.be",
	"x.com",
	"twitter.com",
	"tiktok.com",
}

func ValidateMediaURL(raw string) error {
	if len(raw) == 0 || len(raw) > 2048 {
		return ErrInvalidURL
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return ErrInvalidURL
	}

	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" || net.ParseIP(host) != nil {
		return ErrInvalidURL
	}
	for _, allowed := range allowedHosts {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return nil
		}
	}
	return ErrInvalidURL
}

func ValidateFormatID(value string, allowMerge bool) error {
	if value == "" || len(value) > 128 {
		return errors.New("invalid format ID")
	}
	parts := strings.Split(value, "+")
	if len(parts) != 1 && !allowMerge {
		return errors.New("separate video and audio formats are not supported")
	}
	if len(parts) > 2 {
		return errors.New("at most one video and one audio format can be merged")
	}
	for _, part := range parts {
		if part == "" || strings.HasPrefix(part, "-") {
			return errors.New("invalid format ID")
		}
		for _, char := range part {
			if (char < 'a' || char > 'z') &&
				(char < 'A' || char > 'Z') &&
				(char < '0' || char > '9') &&
				char != '_' && char != '-' && char != '.' {
				return errors.New("invalid format ID")
			}
		}
	}
	return nil
}
