package main

import (
	"os"
	"regexp"
	"testing"
)

var serviceVersionPattern = regexp.MustCompile(`(?m)^version:\s*["']?([^"'\s#]+)["']?\s*(?:#.*)?$`)

func TestVersionMatchesServiceMetadata(t *testing.T) {
	data, err := os.ReadFile("service.yaml")
	if err != nil {
		t.Fatal(err)
	}
	matches := serviceVersionPattern.FindAllSubmatch(data, -1)
	if len(matches) != 1 || len(matches[0]) != 2 {
		t.Fatal("service.yaml must declare exactly one parseable version")
	}
	if got := string(matches[0][1]); got != Version {
		t.Fatalf("binary version %q does not match service.yaml version %q", Version, got)
	}
}

func TestServiceVersionPatternRejectsMissingMetadata(t *testing.T) {
	for _, input := range [][]byte{
		[]byte("name: service\n"),
		[]byte("version:\n"),
		[]byte("version: # not set\n"),
	} {
		if matches := serviceVersionPattern.FindSubmatch(input); len(matches) != 0 {
			t.Fatalf("accepted missing version metadata %q", input)
		}
	}
}
