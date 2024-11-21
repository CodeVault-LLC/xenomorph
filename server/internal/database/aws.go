package database

import (
	"context"
	"errors"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithyendpoints "github.com/aws/smithy-go/endpoints"
	"github.com/google/uuid"
)

var S3Client *s3.Client

type awsEndpointResolver struct {
}

func (*awsEndpointResolver) ResolveEndpoint(ctx context.Context, params s3.EndpointParameters) (
	smithyendpoints.Endpoint, error,
) {
	return s3.NewDefaultEndpointResolverV2().ResolveEndpoint(ctx, params)
}

// InitAWS initializes the AWS S3 client and ensures specified buckets exist.
func InitAWS() error {
	cfg, err := awsConfig.LoadDefaultConfig(context.TODO(),
		awsConfig.WithRegion("us-east-1"),
		awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			"admin",    // Access Key ID (matches MINIO_ROOT_USER)
			"admin123", // Secret Access Key (matches MINIO_ROOT_PASSWORD)
			"",         // Session Token (not needed for MinIO)
		)),
	)

	if err != nil {
		log.Printf("Error loading AWS config: %v", err)
		return err
	}

	S3Client = s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
		o.BaseEndpoint = aws.String("http://localhost:9000")
		o.EndpointResolverV2 = &awsEndpointResolver{}
	})

	if _, err := S3Client.ListBuckets(context.TODO(), &s3.ListBucketsInput{}); err != nil {
		log.Printf("Failed to connect to S3: %v", err)
		return err
	}

	requiredBuckets := []string{"content-bucket"}
	for _, bucket := range requiredBuckets {
		if err := ensureBucketExists(bucket); err != nil {
			log.Printf("Error ensuring bucket %s exists: %v", bucket, err)
			return err
		}
	}

	return nil
}

// ensureBucketExists checks if a bucket exists and creates it if not.
func ensureBucketExists(bucketName string) error {
	_, err := S3Client.HeadBucket(context.TODO(), &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		var noSuchBucket *types.NotFound
		if ok := errors.As(err, &noSuchBucket); ok {
			_, err = S3Client.CreateBucket(context.TODO(), &s3.CreateBucketInput{
				Bucket: aws.String(bucketName),
			})
			if err != nil {
				return err
			}
			log.Printf("Bucket %s created successfully", bucketName)
		} else {
			return err
		}
	}

	return nil
}

func GenerateBucketName(file_extension string) string {
	id := uuid.New().String()
	return id + "." + file_extension
}
