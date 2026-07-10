package storage

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/gophercloud/gophercloud"
	goopenstack "github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/containers"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/objects"

	"github.com/KHU-RETURN/rcp-server/internal/infrastructure/openstack"
)

func isNotFound(err error) bool {
	var e gophercloud.ErrUnexpectedResponseCode
	return errors.As(err, &e) && e.Actual == http.StatusNotFound
}

type Client struct {
	provider *gophercloud.ProviderClient
}

type responseHeaderWriter interface {
	Header() http.Header
}

func NewClient(provider *gophercloud.ProviderClient) *Client {
	return &Client{provider: provider}
}

func (c *Client) serviceClient() (*gophercloud.ServiceClient, error) {
	return goopenstack.NewObjectStorageV1(c.provider, gophercloud.EndpointOpts{
		Region: openstack.Region,
	})
}

func (c *Client) CreateContainer(name string) error {
	sc, err := c.serviceClient()
	if err != nil {
		return err
	}
	return containers.Create(sc, name, nil).Err
}

func (c *Client) DeleteContainer(name string) error {
	sc, err := c.serviceClient()
	if err != nil {
		return err
	}
	err = containers.Delete(sc, name).Err
	if isNotFound(err) {
		return nil
	}
	return err
}

func (c *Client) ListObjects(containerName string) ([]ObjectInfo, error) {
	sc, err := c.serviceClient()
	if err != nil {
		return nil, err
	}
	pages, err := objects.List(sc, containerName, objects.ListOpts{Full: true}).AllPages()
	if err != nil {
		if isNotFound(err) {
			return nil, nil // 컨테이너가 이미 삭제된 경우 빈 목록 반환
		}
		return nil, err
	}
	raw, err := objects.ExtractInfo(pages)
	if err != nil {
		return nil, err
	}
	result := make([]ObjectInfo, len(raw))
	for i, o := range raw {
		result[i] = ObjectInfo{
			Name:         o.Name,
			ContentType:  o.ContentType,
			SizeBytes:    int64(o.Bytes),
			LastModified: o.LastModified,
		}
	}
	return result, nil
}

func (c *Client) UploadObject(containerName, objectName string, r io.Reader, contentType string) error {
	sc, err := c.serviceClient()
	if err != nil {
		return err
	}
	return objects.Create(sc, containerName, objectName, objects.CreateOpts{
		Content:     r,
		ContentType: contentType,
	}).Err
}

func (c *Client) DownloadObject(containerName, objectName string, w io.Writer) error {
	sc, err := c.serviceClient()
	if err != nil {
		return err
	}
	result := objects.Download(sc, containerName, objectName, nil)
	if result.Err != nil {
		return result.Err
	}
	defer func() { _ = result.Body.Close() }()
	copyDownloadHeaders(w, result.Header, objectName)
	if _, err := io.Copy(w, result.Body); err != nil {
		return fmt.Errorf("stream error: %w", err)
	}
	return nil
}

func (c *Client) DeleteObject(containerName, objectName string) error {
	sc, err := c.serviceClient()
	if err != nil {
		return err
	}
	err = objects.Delete(sc, containerName, objectName, nil).Err
	if isNotFound(err) {
		return nil
	}
	return err
}

func (c *Client) BulkDeleteObjects(containerName string, names []string) error {
	sc, err := c.serviceClient()
	if err != nil {
		return err
	}
	return objects.BulkDelete(sc, containerName, names).Err
}

func copyDownloadHeaders(w io.Writer, headers http.Header, objectName string) {
	hw, ok := w.(responseHeaderWriter)
	if !ok {
		return
	}

	if headers != nil {
		for _, name := range []string{
			"Content-Type",
			"Content-Length",
			"Content-Disposition",
			"Content-Encoding",
			"ETag",
			"Last-Modified",
		} {
			if value := headers.Get(name); value != "" {
				hw.Header().Set(name, value)
			}
		}
	}

	if hw.Header().Get("Content-Type") == "" {
		hw.Header().Set("Content-Type", defaultObjectContentType)
	}
	hw.Header().Set("X-Content-Type-Options", "nosniff")
	if shouldForceAttachment(headers.Get("Content-Type"), objectName) {
		hw.Header().Set("Content-Disposition", attachmentContentDisposition(objectName))
	}
}

func shouldForceAttachment(contentType, objectName string) bool {
	if isActiveContentType(contentType) {
		return true
	}

	switch strings.ToLower(path.Ext(objectName)) {
	case ".html", ".htm", ".svg":
		return true
	default:
		return false
	}
}

func isActiveContentType(contentType string) bool {
	contentType = strings.TrimSpace(smartQuoteReplacer.Replace(contentType))
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType, _, _ = strings.Cut(contentType, ";")
	}

	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "text/html", "application/xhtml+xml", "image/svg+xml":
		return true
	default:
		return false
	}
}

func attachmentContentDisposition(objectName string) string {
	filename := path.Base(strings.TrimSpace(objectName))
	if filename == "" || filename == "." || filename == "/" {
		filename = "download"
	}
	if value := mime.FormatMediaType("attachment", map[string]string{"filename": filename}); value != "" {
		return value
	}
	return "attachment"
}
