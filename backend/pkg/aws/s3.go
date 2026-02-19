package aws

import (
    "context"
    "fmt"
    "time"

    sdkaws "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/s3"
)

// NewS3Client creates a new S3 client from AWS config.
func NewS3Client(cfg sdkaws.Config) *s3.Client {
    return s3.NewFromConfig(cfg)
}

// GeneratePresignedPutURL generates a presigned PUT URL for the provided bucket/key.
func GeneratePresignedPutURL(ctx context.Context, cfg sdkaws.Config, bucket, key string, expirySeconds int64) (string, map[string]string, error) {
    client := NewS3Client(cfg)
    presigner := s3.NewPresignClient(client)

    input := &s3.PutObjectInput{
        Bucket: &bucket,
        Key:    &key,
    }

    opts := func(o *s3.PresignOptions) {
        o.Expires = time.Duration(expirySeconds) * time.Second
    }

    presigned, err := presigner.PresignPutObject(ctx, input, opts)
    if err != nil {
        return "", nil, fmt.Errorf("failed to presign put object: %w", err)
    }

    headers := make(map[string]string)
    for k, v := range presigned.SignedHeader {
        if len(v) > 0 {
            headers[k] = v[0]
        }
    }

    return presigned.URL, headers, nil
}
