package s3

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"video/services/worker/internal/processing/ports"
)

type Storage struct {
	client *minio.Client
	bucket string
}

func New(endpoint, region, accessKey, secretKey, bucket string, pathStyle bool) (*Storage, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.Scheme == "" {
		return nil, fmt.Errorf("invalid object storage endpoint")
	}
	options := &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Region: region,
		Secure: parsed.Scheme == "https",
	}
	if pathStyle {
		options.BucketLookup = minio.BucketLookupPath
	}
	client, err := minio.New(parsed.Host, options)
	if err != nil {
		return nil, fmt.Errorf("create object storage client: %w", err)
	}
	return &Storage{client: client, bucket: bucket}, nil
}

func (storage *Storage) Download(ctx context.Context, objectKey string, destination io.Writer) (ports.ObjectInfo, error) {
	object, err := storage.client.GetObject(ctx, storage.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return ports.ObjectInfo{}, mapStorageError(err)
	}
	defer object.Close()

	info, err := object.Stat()
	if err != nil {
		return ports.ObjectInfo{}, mapStorageError(err)
	}
	if _, err := io.Copy(destination, object); err != nil {
		return ports.ObjectInfo{}, fmt.Errorf("%w: %v", ports.ErrObjectUnreadable, err)
	}
	return ports.ObjectInfo{Size: info.Size, ContentType: info.ContentType}, nil
}

func mapStorageError(err error) error {
	if err == nil {
		return nil
	}
	response := minio.ToErrorResponse(err)
	if response.Code == "NoSuchKey" || response.Code == "NoSuchBucket" || strings.Contains(strings.ToLower(err.Error()), "not found") {
		return fmt.Errorf("%w: %v", ports.ErrObjectNotFound, err)
	}
	return err
}

var _ ports.SourceStorage = (*Storage)(nil)
