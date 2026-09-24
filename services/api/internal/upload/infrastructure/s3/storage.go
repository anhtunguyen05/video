package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"video/services/api/internal/upload/ports"
)

type Storage struct {
	client *minio.Client
	bucket string
	region string
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
	return &Storage{client: client, bucket: bucket, region: region}, nil
}

func NewWithClient(client *minio.Client, bucket string) *Storage {
	return &Storage{client: client, bucket: bucket}
}

func (storage *Storage) EnsureBucket(ctx context.Context) error {
	exists, err := storage.client.BucketExists(ctx, storage.bucket)
	if err != nil {
		return mapStorageError(err)
	}
	if exists {
		return nil
	}
	return storage.client.MakeBucket(ctx, storage.bucket, minio.MakeBucketOptions{Region: storage.region})
}

func (storage *Storage) PresignPut(ctx context.Context, objectKey, contentType string, _ int64, expiry time.Duration) (ports.PresignedUpload, error) {
	if expiry <= 0 {
		return ports.PresignedUpload{}, errors.New("presign expiry must be positive")
	}
	presigned, err := storage.client.PresignedPutObject(ctx, storage.bucket, objectKey, expiry)
	if err != nil {
		return ports.PresignedUpload{}, err
	}
	return ports.PresignedUpload{
		URL:       presigned.String(),
		Method:    "PUT",
		Headers:   map[string]string{"Content-Type": contentType},
		ExpiresAt: time.Now().UTC().Add(expiry),
	}, nil
}

func (storage *Storage) StatObject(ctx context.Context, objectKey string) (ports.ObjectInfo, error) {
	info, err := storage.client.StatObject(ctx, storage.bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		return ports.ObjectInfo{}, mapStorageError(err)
	}
	return ports.ObjectInfo{Size: info.Size, ContentType: info.ContentType}, nil
}

func (storage *Storage) ReadObjectPrefix(ctx context.Context, objectKey string, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, errors.New("object prefix limit must be positive")
	}
	object, err := storage.client.GetObject(ctx, storage.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, mapStorageError(err)
	}
	defer object.Close()
	data, err := io.ReadAll(io.LimitReader(object, limit))
	if err != nil {
		return nil, mapStorageError(err)
	}
	return data, nil
}

func (storage *Storage) DeleteObject(ctx context.Context, objectKey string) error {
	if err := storage.client.RemoveObject(ctx, storage.bucket, objectKey, minio.RemoveObjectOptions{}); err != nil {
		return mapStorageError(err)
	}
	return nil
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

var _ ports.ObjectStorage = (*Storage)(nil)
