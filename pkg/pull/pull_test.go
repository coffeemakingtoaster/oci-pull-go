package pull

import (
	"reflect"
	"testing"
)

func TestParseImageWithoutExplicitRegistry(t *testing.T) {
	expected := ImageData{
		registry:  "https://registry-1.docker.io",
		imageName: "library/debian",
		tag:       "latest",
	}
	input := "debian:latest"
	actual := parseImage(input)
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Parse result mismatch: Expected %v Got %v", expected, actual)
	}
}

func TestParseImageWithExplicitRegistry(t *testing.T) {
	expected := ImageData{
		registry:  "https://registry.io",
		imageName: "name/uwubuntu",
		tag:       "newest",
	}
	input := "registry.io/name/uwubuntu:newest"
	actual := parseImage(input)
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Parse result mismatch: Expected %v Got %v", expected, actual)
	}
}
