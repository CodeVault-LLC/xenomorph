package database

import (
	"bytes"
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// UploadFile uploads a file to the specified bucket.
func UploadFile(bucketName string, objectKey string, fileContents []byte, readable bool) error {
	if readable {
		_, err := S3Client.PutObject(context.TODO(), &s3.PutObjectInput{
			Bucket: &bucketName,
			Key:    &objectKey,
			Body:   bytes.NewReader(fileContents),
			ACL:    types.ObjectCannedACLPublicRead,
		})

		return err
	} else {
		_, err := S3Client.PutObject(context.TODO(), &s3.PutObjectInput{
			Bucket: &bucketName,
			Key:    &objectKey,
			Body:   bytes.NewReader(fileContents),
		})

		return err
	}
}

func UploadFileChunks(bucketName, objectKey string, fileContents []byte, chunkSize int64) error {
	ctx := context.TODO()

	initResp, err := S3Client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket: &bucketName,
		Key:    &objectKey,
	})
	if err != nil {
		return fmt.Errorf("failed to initiate multipart upload: %w", err)
	}
	uploadID := *initResp.UploadId
	log.Printf("Initiated multipart upload with UploadId: %s", uploadID)

	var completedParts []types.CompletedPart
	var start, end int64

	// Step 2: Upload each chunk
	for start = 0; start < int64(len(fileContents)); start += chunkSize {
		if start+chunkSize < int64(len(fileContents)) {
			end = start + chunkSize
		} else {
			end = int64(len(fileContents))
		}

		partNumber := int32(start/chunkSize) + 1
		log.Printf("Uploading part %d...", partNumber)

		uploadResp, err := S3Client.UploadPart(ctx, &s3.UploadPartInput{
			Bucket:     &bucketName,
			Key:        &objectKey,
			UploadId:   &uploadID,
			Body:       bytes.NewReader(fileContents[start:end]),
			PartNumber: &partNumber,
		})
		if err != nil {
			// Abort the multipart upload on failure
			_, abortErr := S3Client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
				Bucket:   &bucketName,
				Key:      &objectKey,
				UploadId: &uploadID,
			})
			if abortErr != nil {
				log.Printf("Failed to abort multipart upload: %v", abortErr)
			}
			return fmt.Errorf("failed to upload part %d: %w", partNumber, err)
		}

		// Save the ETag and part number
		completedParts = append(completedParts, types.CompletedPart{
			ETag:       uploadResp.ETag,
			PartNumber: &partNumber,
		})
		log.Printf("Successfully uploaded part %d", partNumber)
	}

	// Step 3: Complete multipart upload
	_, err = S3Client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   &bucketName,
		Key:      &objectKey,
		UploadId: &uploadID,
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	log.Printf("Successfully completed multipart upload for %s/%s", bucketName, objectKey)
	return nil
}

// DownloadFile downloads a file from the specified bucket.
func DownloadFile(bucketName string, objectKey string) ([]byte, error) {
	resp, err := S3Client.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: &bucketName,
		Key:    &objectKey,
	})
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(resp.Body)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// DeleteFile deletes a file from the specified bucket.
func DeleteFile(bucketName string, objectKey string) error {
	_, err := S3Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: &bucketName,
		Key:    &objectKey,
	})
	return err
}

// ListFiles lists all files in the specified bucket.
func ListFiles(bucketName string) ([]string, error) {
	resp, err := S3Client.ListObjectsV2(context.TODO(), &s3.ListObjectsV2Input{
		Bucket: &bucketName,
	})
	if err != nil {
		return nil, err
	}

	var keys []string
	for _, obj := range resp.Contents {
		keys = append(keys, *obj.Key)
	}

	return keys, nil
}

// GetEndpoint returns the endpoint for the specified bucket.
func GetEndpoint(bucketName string) string {
	return "http://localhost:9000/" + bucketName
}

// GetLocation returns the location of the specified object placed in the specified bucket.
func GetLocation(bucketName, objectKey string) string {
	return "http://localhost:9000/" + bucketName + "/" + objectKey
}
