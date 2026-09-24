package config

import "testing"

func TestLoadReadsUploadConfiguration(t *testing.T) {
	t.Setenv("API_PORT", "9090")
	t.Setenv("S3_USE_PATH_STYLE", "false")
	t.Setenv("UPLOAD_URL_EXPIRY", "10m")
	t.Setenv("MAX_UPLOAD_SIZE_BYTES", "2048")
	t.Setenv("ALLOWED_UPLOAD_CONTENT_TYPES", "video/mp4, video/webm")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != ":9090" || cfg.S3UsePathStyle || cfg.UploadURLExpiry.String() != "10m0s" || cfg.MaxUploadSizeBytes != 2048 {
		t.Fatalf("unexpected upload config: %#v", cfg)
	}
	if len(cfg.AllowedUploadMimeTypes) != 2 || cfg.AllowedUploadMimeTypes[1] != "video/webm" {
		t.Fatalf("unexpected MIME types: %#v", cfg.AllowedUploadMimeTypes)
	}
}

func TestLoadRejectsInvalidUploadConfiguration(t *testing.T) {
	t.Setenv("MAX_UPLOAD_SIZE_BYTES", "not-a-number")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want invalid upload size error")
	}
}
