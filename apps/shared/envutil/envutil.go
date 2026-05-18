package envutil

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
)

func MustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "required env var %s is not set\n", key)
		os.Exit(1)
	}
	return v
}

func GetEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func ParseLogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func DecodePEMFromEnv(key string) []byte {
	val := MustEnv(key)
	b, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decode env %s: %v\n", key, err)
		os.Exit(1)
	}
	return b
}
