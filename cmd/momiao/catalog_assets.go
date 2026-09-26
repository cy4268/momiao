package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cy4268/momiao/internal/platform"
)

type catalogAssetConfig struct{ AccountID, Bucket, CredentialsFile string }
type catalogAssetStore struct {
	client *s3.Client
	bucket string
}

var errCatalogAssetConfig = errors.New("invalid catalog asset configuration")
var errCatalogAssetUpload = errors.New("CATALOG_ASSET_UPLOAD_FAILED")

func loadCatalogAssetConfig(lookup func(string) (string, bool)) (catalogAssetConfig, error) {
	var c catalogAssetConfig
	c.AccountID, _ = lookup("MOMIAO_CATALOG_ASSET_R2_ACCOUNT_ID")
	c.Bucket, _ = lookup("MOMIAO_CATALOG_ASSET_R2_BUCKET")
	c.CredentialsFile, _ = lookup("MOMIAO_CATALOG_ASSET_R2_CREDENTIALS_FILE")
	if c == (catalogAssetConfig{}) {
		return c, nil
	}
	if !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(c.AccountID) || !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`).MatchString(c.Bucket) || !filepath.IsAbs(c.CredentialsFile) {
		return catalogAssetConfig{}, errCatalogAssetConfig
	}
	if _, _, err := readCatalogAssetCredentials(c.CredentialsFile); err != nil {
		return catalogAssetConfig{}, err
	}
	return c, nil
}
func readCatalogAssetCredentials(path string) (string, string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return "", "", errCatalogAssetConfig
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 4096 {
		return "", "", errCatalogAssetConfig
	}
	var key struct {
		AccessKeyID     string `json:"access_key_id"`
		SecretAccessKey string `json:"secret_access_key"`
	}
	valid := regexp.MustCompile(`^[A-Za-z0-9_+/=-]{8,256}$`)
	if !decodeAnnouncementBody(bytes.NewReader(data), &key) || !valid.MatchString(key.AccessKeyID) || !valid.MatchString(key.SecretAccessKey) {
		return "", "", errCatalogAssetConfig
	}
	return key.AccessKeyID, key.SecretAccessKey, nil
}
func newCatalogAssetStore(c catalogAssetConfig, client *http.Client) (*catalogAssetStore, error) {
	if c == (catalogAssetConfig{}) {
		return nil, nil
	}
	if !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(c.AccountID) || !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`).MatchString(c.Bucket) || !filepath.IsAbs(c.CredentialsFile) {
		return nil, errCatalogAssetConfig
	}
	id, secret, err := readCatalogAssetCredentials(c.CredentialsFile)
	if err != nil {
		return nil, err
	}
	bounded := http.Client{Timeout: 60 * time.Second}
	if client != nil {
		bounded = *client
		if bounded.Timeout <= 0 || bounded.Timeout > 60*time.Second {
			bounded.Timeout = 60 * time.Second
		}
	}
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	sdk := s3.NewFromConfig(aws.Config{Region: "auto", Credentials: credentials.NewStaticCredentialsProvider(id, secret, ""), HTTPClient: &bounded, RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired, Retryer: func() aws.Retryer {
		return retry.NewStandard(func(o *retry.StandardOptions) { o.MaxAttempts = 2; o.MaxBackoff = time.Second })
	}}, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("https://" + c.AccountID + ".r2.cloudflarestorage.com")
		o.UsePathStyle = true
	})
	return &catalogAssetStore{client: sdk, bucket: c.Bucket}, nil
}
func (s *catalogAssetStore) Put(ctx context.Context, family string, f catalogCoverFile) error {
	key, err := platform.CatalogCoverObjectKey(family, f.Image.SHA256, f.Image.Extension)
	if err != nil {
		return err
	}
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key), Body: bytes.NewReader(f.Bytes), ContentLength: aws.Int64(int64(len(f.Bytes))), ContentType: aws.String(f.Image.ContentType), CacheControl: aws.String("public, max-age=31536000, immutable")})
	if err != nil {
		return errCatalogAssetUpload
	}
	return nil
}
