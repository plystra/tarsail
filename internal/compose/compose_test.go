package compose

import (
	"strings"
	"testing"
)

func TestDiscoverServiceImages(t *testing.T) {
	output := `services:
  web:
    build:
      context: ./web
    image: my-app-web:local
  api:
    image: my-app-api:local
`
	images, err := DiscoverServiceImages(output)
	if err != nil {
		t.Fatalf("DiscoverServiceImages returned error: %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("len(images) = %d", len(images))
	}
	if images[0].Service != "api" || images[0].File != "images/api.tar" {
		t.Fatalf("unexpected first image: %+v", images[0])
	}
}

func TestDiscoverServiceImagesRejectsBuildWithoutImage(t *testing.T) {
	output := `services:
  api:
    build:
      context: ./api
`
	_, err := DiscoverServiceImages(output)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "build section but no explicit image tag") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDiscoverServiceImagesRejectsImageLessService(t *testing.T) {
	output := `services:
  redis:
    ports:
      - "6379:6379"
`
	_, err := DiscoverServiceImages(output)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "has no image tag") {
		t.Fatalf("unexpected error: %v", err)
	}
}
