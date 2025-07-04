package registry

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/coffeemakingtoaster/oci-pull-go/pkg/oci"
	"github.com/rs/zerolog/log"
)

var knownAuthVariations = map[string]string{
	"https://registry-1.docker.io": "https://auth.docker.io",
}

type OCIDownloader struct {
	image                   string
	tag                     string
	destination             string
	token                   string
	registryApiEndpointAuth string
	registryApiEndpointV2   string
}

func (od *OCIDownloader) ToString() string {
	return fmt.Sprintf("Image: %s Tag: %s Registry: %s", od.image, od.tag, od.registryApiEndpointV2)
}

func isValid(apiEndpoint string) bool {
	res, err := http.Get(apiEndpoint)
	log.Debug().Int("status code", res.StatusCode).Send()
	return res.StatusCode != 200 || err != nil
}

func newOciDownloader(registry, image, tag, destination string) *OCIDownloader {
	apiEndpointV2 := fmt.Sprintf("%s/v2", registry)
	if !isValid(apiEndpointV2) {
		log.Error().Msgf("Api endpoint is not valid: %s", apiEndpointV2)
		return nil
	}
	// check for known differences in api design
	// otherwise just assume that we can use the base registry
	authEndpoint, ok := knownAuthVariations[registry]
	if !ok {
		authEndpoint = registry
	}
	return &OCIDownloader{
		image:                   image,
		tag:                     tag,
		destination:             destination,
		registryApiEndpointV2:   apiEndpointV2,
		registryApiEndpointAuth: authEndpoint,
	}
}

// For now this ONLY supports ghcr
func DownloadOciToPath(registry, image, tag, destination string) error {
	downloader := newOciDownloader(registry, image, tag, destination)
	if downloader == nil {
		return errors.New("Could not parse provided image, see logs for details ")
	}
	manifest, err := downloader.GetManifest()
	if err != nil {
		return err
	}
	// default to first
	wanted := manifest.Manifests[0]
	for _, v := range manifest.Manifests {
		if v.Platform.OS == "linux" {
			wanted = v
		}
	}
	man, _ := downloader.GetSpecificManifest(wanted.Digest)
	writer := downloader.openTar()
	defer writer.Close()
	for i, v := range man.Layers {
		log.Debug().Int("Current layer", i).Int("total layers", len(man.Layers)).Msg("Pulling image")
		err := downloader.addLayerToTar(writer, v)
		if err != nil {
			log.Error().Err(err).Msg("Could not add layer to tar due to an error")
		}
	}
	manifestMetadata := tar.Header{
		Name:    fmt.Sprintf("blobs/%s", strings.Replace(wanted.Digest, ":", "/", 1)),
		Mode:    0644,
		ModTime: time.Now(),
	}
	err = writeStructToTar(writer, &manifestMetadata, man)
	if err != nil {
		log.Error().Err(err).Msg("Could not write manifest to tar")
	}
	indexMetadata := tar.Header{
		Name:    "index.json",
		Mode:    0644,
		ModTime: time.Now(),
	}
	// only show manifest that was downloaded
	manifest.Manifests = []oci.Manifest{wanted}
	err = writeStructToTar(writer, &indexMetadata, manifest)
	if err != nil {
		log.Error().Err(err).Msg("Could not write index to tar")
	}
	err = downloader.addConfigToTar(writer, man.Config.Digest)
	if err != nil {
		log.Error().Err(err).Msg("Could not write metadata to tar")
	}
	return nil
}

func (od *OCIDownloader) RefreshToken() {
	registryBaseUrl := ""
	base, err := url.Parse(od.registryApiEndpointV2)
	if err != nil {
		log.Error().Err(err)
	} else {
		registryBaseUrl = base.Host
	}
	if registryBaseUrl == "registry-1.docker.io" {
		registryBaseUrl = "registry.docker.io"
	}
	res, err := http.Get(
		fmt.Sprintf("%s/token?service=%s&scope=repository:%s:pull", od.registryApiEndpointAuth, registryBaseUrl, od.image),
	)

	if err != nil {
		log.Error().Err(err).Msg("Could not fetch auth token for oci repository")
		return
	}
	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		log.Error().Err(err).Msg("Could not fetch auth token for oci repository")
		return
	}
	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		log.Error().Err(err).Str("data", string(data)).Msg("Could not fetch auth token for oci repository")
		return
	}
	val, ok := parsed["token"]
	if !ok {
		log.Error().Msgf("No token in repository response: %v", parsed)
		return
	}
	od.token = val.(string)
	log.Debug().Msg("OCI Downloader auth token refreshed")
}

func (od *OCIDownloader) doRequest(url, acceptHeader string) ([]byte, error) {
	if od.token == "" {
		od.RefreshToken()
		if od.token == "" {
			return nil, errors.New("Could not refresh token, check logs for details")
		}
		return od.doRequest(url, acceptHeader)
	}
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return []byte{}, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", od.token))
	req.Header.Add("Accept", acceptHeader)
	res, err := client.Do(req)
	if err != nil {
		return []byte{}, err
	}
	defer res.Body.Close()
	return io.ReadAll(res.Body)
}

func (od *OCIDownloader) GetManifest() (oci.OCIImageIndex, error) {
	data, err := od.doRequest(fmt.Sprintf("%s/%s/manifests/%s", od.registryApiEndpointV2, od.image, od.tag), "application/vnd.oci.image.manifest.v1+json, application/vnd.oci.image.index.v1+json")
	if err != nil {
		log.Error().Err(err).Msg("Could not fetch manifest for image")
		return oci.OCIImageIndex{}, err
	}

	var parsed oci.OCIImageIndex

	log.Debug().Msg(fmt.Sprintf("%s/%s/manifests/%s", od.registryApiEndpointV2, od.image, od.tag))
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		log.Error().Err(err).Msg("Could not fetch auth token for oci repository")
		return oci.OCIImageIndex{}, err
	}
	log.Debug().Msg("OCI image manifest fetched")
	return parsed, nil
}

func (od *OCIDownloader) GetSpecificManifest(digest string) (oci.OCIImageManifest, error) {
	data, err := od.doRequest(fmt.Sprintf("%s/%s/manifests/%s", od.registryApiEndpointV2, od.image, digest), "application/vnd.oci.image.manifest.v1+json, application/vnd.oci.image.index.v1+json")
	if err != nil {
		log.Error().Err(err).Msg("Could not fetch manifest for image")
		return oci.OCIImageManifest{}, err
	}

	var parsed oci.OCIImageManifest
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		log.Error().Err(err).Msg("Could not fetch auth token for oci repository")
		return oci.OCIImageManifest{}, err
	}
	log.Debug().Msg("OCI image manifest fetched")
	return parsed, nil

}

func (od *OCIDownloader) openTar() *tar.Writer {
	// TODO: Error handling
	file, err := os.Create(od.destination)
	if err != nil {
		log.Error().Err(err).Msg("Could not create tar file")
	}
	writer := tar.NewWriter(file)
	return writer
}

func (od *OCIDownloader) getLayerData(digest string) ([]byte, error) {
	return od.doRequest(fmt.Sprintf("%s/%s/blobs/%s", od.registryApiEndpointV2, od.image, digest), "")
}

func writeToTar(writer *tar.Writer, header *tar.Header, data []byte) error {
	if err := writer.WriteHeader(header); err != nil {
		return err
	}

	buff := bytes.NewBuffer(data)

	_, err := io.Copy(writer, buff)
	if err != nil {
		return err
	}
	return nil
}

func (od *OCIDownloader) addLayerToTar(writer *tar.Writer, metadata oci.LayerMetaData) error {
	data, err := od.getLayerData(metadata.Digest)
	if err != nil {
		return err
	}
	name := strings.Replace(metadata.Digest, ":", "/", 1)
	header := &tar.Header{
		Name:    fmt.Sprintf("blobs/%s", name),
		Mode:    0644,
		Size:    int64(metadata.Size),
		ModTime: time.Now(),
	}
	return writeToTar(writer, header, data)
}

func (od *OCIDownloader) addConfigToTar(writer *tar.Writer, digest string) error {
	data, err := od.doRequest(fmt.Sprintf("%s/%s/blobs/%s", od.registryApiEndpointV2, od.image, digest), "")

	if err != nil {
		return err
	}

	var cfg oci.ImageMetadata

	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return err
	}

	header := tar.Header{
		Name:    fmt.Sprintf("blobs/%s", strings.Replace(digest, ":", "/", 1)),
		Mode:    0644,
		ModTime: time.Now(),
	}
	return writeStructToTar(writer, &header, cfg)
}

func writeStructToTar[T any](writer *tar.Writer, header *tar.Header, data T) error {
	content, err := json.Marshal(data)
	if err != nil {
		return err
	}
	header.Size = int64(len(content))
	return writeToTar(writer, header, content)
}
