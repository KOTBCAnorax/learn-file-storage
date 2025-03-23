package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		fmt.Println("Failed to unmarshal")
		return "", err
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
