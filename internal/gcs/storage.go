package gcs

import (
	"cloud.google.com/go/storage"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
)

type Client struct {
	bucket *storage.BucketHandle
	base   string
}

func NewClient(base string) *Client {
	c, err := storage.NewClient(context.Background())
	if err != nil {
		panic(err.Error())
	}
	b, rest, _ := strings.Cut(base, "/")
	bucket := c.Bucket(b)
	return &Client{
		bucket: bucket,
		base:   rest,
	}
}

func Fetch[T any](c *Client, path string) (T, error) {
	var res T
	reader, err := c.bucket.Object(filepath.Join(c.base, path)).NewReader(context.Background())
	if err != nil {
		return res, err
	}
	if err := json.NewDecoder(reader).Decode(&res); err != nil {
		return res, err
	}
	return res, nil
}
