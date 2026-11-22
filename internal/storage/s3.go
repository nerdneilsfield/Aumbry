package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/logging"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// S3Options configures an S3-compatible object store and its transport timeouts.
type S3Options struct {
	Endpoint              string
	Region                string
	Bucket                string
	AccessKeyID           string
	SecretAccessKey       string
	PathStyle             bool
	DialTimeout           time.Duration
	ResponseHeaderTimeout time.Duration
	ProbeTimeout          time.Duration
	// ListPageSize overrides S3 ListObjectsV2 MaxKeys (used by pagination tests); 0 uses the S3 default.
	ListPageSize int32
}

// S3 stores objects in an S3-compatible bucket (AWS S3, Cloudflare R2, MinIO, etc.).
type S3 struct {
	client       *s3.Client
	probeClient  *s3.Client
	bucket       string
	listPageSize int32
}

// ValidateS3Endpoint verifies that a non-empty S3 endpoint uses http:// or https:// and has
// no path, query string, fragment, or embedded userinfo.
func ValidateS3Endpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return errors.New("must use http:// or https:// scheme")
	}
	if u.Host == "" || u.Hostname() == "" {
		return errors.New("must include a host")
	}
	if u.User != nil {
		return errors.New("must not contain userinfo")
	}
	if u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(raw, "?#") {
		return errors.New("must not contain a path, query, or fragment")
	}
	return nil
}

// NewS3 constructs an S3-compatible storage backend with static credentials and bounded timeouts.
func NewS3(opts S3Options) (*S3, error) {
	opts.Endpoint = strings.TrimSpace(opts.Endpoint)
	opts.Region = strings.TrimSpace(opts.Region)
	opts.Bucket = strings.TrimSpace(opts.Bucket)
	opts.AccessKeyID = strings.TrimSpace(opts.AccessKeyID)
	opts.SecretAccessKey = strings.TrimSpace(opts.SecretAccessKey)

	if opts.Bucket == "" {
		return nil, errors.New("s3 bucket is required")
	}
	if opts.AccessKeyID == "" || opts.SecretAccessKey == "" {
		return nil, errors.New("s3 credentials are required")
	}
	if opts.Endpoint != "" {
		if err := ValidateS3Endpoint(opts.Endpoint); err != nil {
			return nil, fmt.Errorf("invalid s3 endpoint: %w", err)
		}
		if opts.Region == "" {
			opts.Region = "auto"
		}
	} else if opts.Region == "" {
		return nil, errors.New("s3 region is required when endpoint is empty")
	}

	if opts.DialTimeout <= 0 {
		opts.DialTimeout = defaultDialTimeout
	}
	if opts.ResponseHeaderTimeout <= 0 {
		opts.ResponseHeaderTimeout = defaultResponseHeaderTimeout
	}
	if opts.ProbeTimeout <= 0 {
		opts.ProbeTimeout = defaultProbeTimeout
	}

	creds := credentials.NewStaticCredentialsProvider(opts.AccessKeyID, opts.SecretAccessKey, "")
	makeClient := func(httpClient *http.Client, maxAttempts int) *s3.Client {
		s3Opts := s3.Options{
			Region:                     opts.Region,
			Credentials:                creds,
			UsePathStyle:               opts.PathStyle,
			HTTPClient:                 httpClient,
			RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
			ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
			DisableLogOutputChecksumValidationSkipped: true,
			Logger: logging.Nop{},
		}
		if opts.Endpoint != "" {
			s3Opts.BaseEndpoint = aws.String(opts.Endpoint)
		}
		if maxAttempts > 0 {
			s3Opts.RetryMaxAttempts = maxAttempts
		}
		return s3.New(s3Opts)
	}

	client := makeClient(&http.Client{
		Transport: newHTTPTransport(opts.DialTimeout, opts.ResponseHeaderTimeout),
	}, 0)
	probeClient := makeClient(&http.Client{
		Transport: newHTTPTransport(opts.ProbeTimeout, opts.ProbeTimeout),
		Timeout:   opts.ProbeTimeout,
	}, 1)

	return &S3{
		client:       client,
		probeClient:  probeClient,
		bucket:       opts.Bucket,
		listPageSize: opts.ListPageSize,
	}, nil
}

// Put uploads body under the S3 key corresponding to name.
func (s *S3) Put(ctx context.Context, name string, body io.ReadSeeker) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if body == nil {
		return errors.New("storage: nil body")
	}
	key, err := normalizeObjectPath(name)
	if err != nil {
		return err
	}
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   body,
	})
	return mapS3WriteErr(err)
}

// Open issues a streaming GetObject and checks errors (including ErrNotFound) before returning the stream.
func (s *S3) Open(ctx context.Context, name string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key, err := normalizeObjectPath(name)
	if err != nil {
		return nil, err
	}
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, mapS3Err(err)
	}
	return out.Body, nil
}

// Exists checks object metadata with HeadObject; 403 maps to ErrAuth while 404 returns (false, nil).
func (s *S3) Exists(ctx context.Context, name string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	key, err := normalizeObjectPath(name)
	if err != nil {
		return false, err
	}
	_, err = s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, nil
	}
	mapped := mapS3Err(err)
	if errors.Is(mapped, ErrNotFound) {
		return false, nil
	}
	return false, mapped
}

// Delete deletes key; a missing object is not an error, whereas missing bucket or auth failures still fail.
func (s *S3) Delete(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key, err := normalizeObjectPath(name)
	if err != nil {
		return err
	}
	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return nil
	}
	// S3 answers 204 for a missing key, so a 404 here comes from a wrong endpoint or proxy and is a failure.
	return mapS3WriteErr(err)
}

// List enumerates all direct objects under dir using ListObjectsV2 pagination with delimiter "/".
func (s *S3) List(ctx context.Context, dir string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rel, err := normalizeDirPath(dir)
	if err != nil {
		return nil, err
	}
	prefix := rel + "/"

	input := &s3.ListObjectsV2Input{
		Bucket:    aws.String(s.bucket),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
	}
	if s.listPageSize > 0 {
		input.MaxKeys = aws.Int32(s.listPageSize)
	}

	pager := s3.NewListObjectsV2Paginator(s.client, input)
	var paths []string
	for pager.HasMorePages() {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, mapS3WriteErr(err)
		}
		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			if key == "" || key == prefix || strings.HasSuffix(key, "/") || strings.HasPrefix(path.Base(key), ".tmp-") {
				continue
			}
			paths = append(paths, "/"+key)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// Probe performs a bounded HeadBucket against the configured bucket and classifies failures.
func (s *S3) Probe(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("%w: %w", ErrTimeout, err)
		}
		return fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
	_, err := s.probeClient.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	})
	if err == nil {
		return nil
	}
	if isS3AuthErr(err) {
		return fmt.Errorf("%w: %s", ErrAuth, s3Cause(err))
	}
	if isS3TimeoutErr(err) {
		return fmt.Errorf("%w: %s", ErrTimeout, s3Cause(err))
	}
	return fmt.Errorf("%w: %s", ErrUnreachable, s3Cause(err))
}

// s3Cause describes err by HTTP status and S3 error code only. The service's free-text message and
// the SDK's request details are dropped because some S3-compatible services echo the Authorization header.
func s3Cause(err error) string {
	parts := []string{"s3 request failed"}
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) {
		parts = append(parts, fmt.Sprintf("status %d", respErr.HTTPStatusCode()))
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode() != "" {
		parts = append(parts, "code "+apiErr.ErrorCode())
	}
	if len(parts) == 1 && isS3TimeoutErr(err) {
		parts = append(parts, "timeout")
	}
	return strings.Join(parts, ", ")
}

func mapS3Err(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case isS3AuthErr(err):
		return fmt.Errorf("%w: %s", ErrAuth, s3Cause(err))
	case isS3TimeoutErr(err):
		return fmt.Errorf("%w: %s", ErrTimeout, s3Cause(err))
	case errors.Is(err, context.Canceled):
		return err
	case isNoSuchBucketErr(err):
		return fmt.Errorf("%w: %s", ErrUnreachable, s3Cause(err))
	case isS3ObjectNotFoundErr(err):
		return ErrNotFound
	}
	return fmt.Errorf("%w: %s", ErrUnreachable, s3Cause(err))
}

// mapS3WriteErr is mapS3Err for operations that never report a missing key (Put, Delete, List):
// a 404 there means the bucket, the endpoint or a proxy in front of it is wrong.
func mapS3WriteErr(err error) error {
	mapped := mapS3Err(err)
	if errors.Is(mapped, ErrNotFound) {
		return fmt.Errorf("%w: %s", ErrUnreachable, s3Cause(err))
	}
	return mapped
}

func isS3AuthErr(err error) bool {
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) {
		code := respErr.HTTPStatusCode()
		if code == http.StatusUnauthorized || code == http.StatusForbidden {
			return true
		}
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "AccessDenied", "InvalidAccessKeyId", "SignatureDoesNotMatch", "ExpiredToken", "InvalidToken", "UnauthorizedAccess":
			return true
		}
	}
	return false
}

func isS3TimeoutErr(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == "RequestTimeout"
}

func isNoSuchBucketErr(err error) bool {
	var nsb *s3types.NoSuchBucket
	if errors.As(err, &nsb) {
		return true
	}
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && strings.EqualFold(apiErr.ErrorCode(), "NoSuchBucket")
}

func isS3ObjectNotFoundErr(err error) bool {
	var nsk *s3types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var nf *s3types.NotFound
	if errors.As(err, &nf) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		if strings.EqualFold(code, "NoSuchKey") || strings.EqualFold(code, "NotFound") {
			return true
		}
	}
	var respErr *smithyhttp.ResponseError
	return errors.As(err, &respErr) && respErr.HTTPStatusCode() == http.StatusNotFound
}
