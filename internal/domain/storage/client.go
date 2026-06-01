package storage

import (
	"fmt"
	"io"
	"net/http"

	"github.com/gophercloud/gophercloud"
	goopenstack "github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/containers"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/objects"

	"github.com/KHU-RETURN/rcp-server/internal/infrastructure/openstack"
)

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
	return containers.Delete(sc, name).Err
}

func (c *Client) ListObjects(containerName string) ([]ObjectInfo, error) {
	sc, err := c.serviceClient()
	if err != nil {
		return nil, err
	}
	pages, err := objects.List(sc, containerName, objects.ListOpts{Full: true}).AllPages()
	if err != nil {
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
	copyDownloadHeaders(w, result.Header)
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
	return objects.Delete(sc, containerName, objectName, nil).Err
}

func (c *Client) BulkDeleteObjects(containerName string, names []string) error {
	sc, err := c.serviceClient()
	if err != nil {
		return err
	}
	return objects.BulkDelete(sc, containerName, names).Err
}

func copyDownloadHeaders(w io.Writer, headers http.Header) {
	hw, ok := w.(responseHeaderWriter)
	if !ok || headers == nil {
		return
	}

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
