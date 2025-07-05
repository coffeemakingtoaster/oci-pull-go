package pull

import (
	"fmt"
	"strings"

	"github.com/coffeemakingtoaster/oci-pull-go/pkg/registry"
	"github.com/rs/zerolog/log"
)

const defaultRegistry = "https://registry-1.docker.io"

// Struct containing all information necessary for pulling an image
type ImageData struct {
	imageName string
	registry  string
	tag       string
}

// Return formatted string representation
func (id *ImageData) ToString() string {
	return fmt.Sprintf("Image: %s Tag: %s Registry: %s", id.imageName, id.tag, id.registry)
}

// Pull given image to path
// Image should be in the forward of [registry.domain/]<imagename>:tag
// Registry should support the v2 docker api. Auth edge cases will (likely) not be handled
// Default repository is docker hub
func PullToPath(image, destinationPath string) error {
	imageData := parseImage(image)
	log.Debug().Msg(imageData.ToString())
	return registry.DownloadOciToPath(imageData.registry, imageData.imageName, imageData.tag, destinationPath)
}

// [registry.domain/]<imagename>:tag
func parseImage(image string) ImageData {
	var imageData ImageData
	tagSplit := strings.Split(image, ":")
	imageData.tag = tagSplit[len(tagSplit)-1]
	imageData.registry = defaultRegistry
	slashSplit := strings.Split(image, "/")
	// use default registry
	// debian:latest
	if len(slashSplit) == 1 {
		imageData.imageName = fmt.Sprintf("library/%s", tagSplit[0])
	} else if strings.Contains(slashSplit[0], ":") || strings.Contains(slashSplit[0], ".") {
		imageData.imageName = strings.Join(slashSplit[1:], "/")
		imageData.registry = slashSplit[0]
	}
	imageData.imageName = strings.TrimSuffix(imageData.imageName, fmt.Sprintf(":%s", imageData.tag))
	imageData.registry = normalizeRegistry(imageData.registry)
	return imageData
}

func normalizeRegistry(base string) string {
	// trim protocol and just add http
	base = strings.TrimPrefix(base, "http://")
	base = strings.TrimPrefix(base, "https://")
	base = fmt.Sprintf("https://%s", base)
	return strings.TrimSuffix(base, "/")
}
