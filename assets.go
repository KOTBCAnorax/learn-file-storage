package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
)

func (cfg apiConfig) ensureAssetsDir() error {
	if _, err := os.Stat(cfg.assetsRoot); os.IsNotExist(err) {
		return os.Mkdir(cfg.assetsRoot, 0755)
	}
	return nil
}

func getAssetPath(videoID, mediaType string) string {
	ext := mediaTypeToExt(mediaType)
	return fmt.Sprintf("%s%s", videoID, ext)
}

func (cfg apiConfig) getAssetDiskPath(assetPath string) string {
	return filepath.Join(cfg.assetsRoot, assetPath)
}

func (cfg apiConfig) getAssetURL(assetPath string) string {
	return fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, assetPath)
}

func mediaTypeToExt(mediaType string) string {
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 {
		return ".bin"
	}
	return "." + parts[1]
}

func (cfg *apiConfig) createVideoURL(name string) string {
	return "https://" + cfg.s3Bucket + ".s3." + cfg.s3Region + ".amazonaws.com/" + name
}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	presignedClient := s3.NewPresignClient(s3Client)
	request, err := presignedClient.PresignGetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	}, s3.WithPresignExpires(expireTime))
	if err != nil {
		return "", err
	}

	return request.URL, nil
}

func getVideoAspectRatio(path string) (string, error) {
	const landscapeRatio = float64(16.0 / 9.0)
	const portraitRatio = float64(9.0 / 16.0)

	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	fileInfo, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if fileInfo.Size() == 0 {
		return "", fmt.Errorf("empty file")
	}

	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("ffprobe error: %w, stderr: %s", err, stderr.String())
	}

	data := FFProbeOutput{}
	err = json.Unmarshal(stdout.Bytes(), &data)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal: %v", err)
	}

	aspectRatio := float64(data.Streams[0].Width) / float64(data.Streams[0].Height)
	if aspectRatio >= landscapeRatio-0.1 && aspectRatio <= landscapeRatio+0.1 {
		return "16:9", nil
	} else if aspectRatio >= portraitRatio-0.1 && aspectRatio <= portraitRatio+0.1 {
		return "9:16", nil
	} else {
		return "other", nil
	}
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	// Check if VideoURL is nil or empty
	if video.VideoURL == nil {
		// Return the video as is for drafts without a video
		return video, nil
	}

	if *video.VideoURL == "" {
		return video, nil
	}

	bucketAndKey := strings.Split(*video.VideoURL, ",")
	if len(bucketAndKey) != 2 {
		return database.Video{}, fmt.Errorf("video url invalid")
	}

	bucket := bucketAndKey[0]
	key := bucketAndKey[1]
	expireTime := 10 * time.Minute

	presignedURL, err := generatePresignedURL(cfg.s3Client, bucket, key, expireTime)
	if err != nil {
		return database.Video{}, fmt.Errorf("failed to generate presigned url: %v", err)
	}
	video.VideoURL = &presignedURL

	return video, nil
}
