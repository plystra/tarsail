package bundle

import "testing"

func TestValidateManifest(t *testing.T) {
	manifest := Manifest{
		SchemaVersion: 1,
		Project:       "my-app",
		ReleaseID:     "20260618-120000-a111",
		CreatedAt:     "2026-06-18T12:00:00Z",
		ComposeFile:   "compose.yaml",
		Images: []ManifestImage{
			{Service: "api", Image: "my-app-api:local", File: "images/api.tar"},
		},
	}
	if err := ValidateManifest(manifest); err != nil {
		t.Fatalf("ValidateManifest returned error: %v", err)
	}
}

func TestValidateManifestRejectsUnsafeImagePath(t *testing.T) {
	manifest := Manifest{
		SchemaVersion: 1,
		Project:       "my-app",
		ReleaseID:     "20260618-120000-a111",
		CreatedAt:     "2026-06-18T12:00:00Z",
		ComposeFile:   "compose.yaml",
		Images: []ManifestImage{
			{Service: "api", Image: "my-app-api:local", File: "../api.tar"},
		},
	}
	if err := ValidateManifest(manifest); err == nil {
		t.Fatal("expected error")
	}
}
