package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	const maxMemory = 1 << 30 // 1 GB
	r.Body = http.MaxBytesReader(w, r.Body, maxMemory)

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't find video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not authorized to update this video", nil)
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-type"))
	if err != nil {
		respondWithError(w, 500, "Something's wrong", err)
		return
	}
	if mediaType == "" {
		respondWithError(w, http.StatusBadRequest, "Missing Content-Type for thumbnail", nil)
		return
	}
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusForbidden, "invalid file type", nil)
		return
	}

	tmp, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		fmt.Printf("error while creating temporary file: %v\n", err)
		respondWithError(w, 500, "Internal error", nil)
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	_, err = io.Copy(tmp, file)
	if err != nil {
		fmt.Printf("error while copying tmp file to memory: %v\n", err)
		respondWithError(w, 500, "Internal error", nil)
		return
	}

	tmp.Seek(0, io.SeekStart)

	aspectRatioPrefix, err := getVideoAspectRatio(tmp.Name())
	if err != nil {
		fmt.Printf("error while getting aspect ratio: %v\n", err)
		respondWithError(w, 500, "Internal error", nil)
		return
	}
	switch aspectRatioPrefix {
	case "16:9":
		aspectRatioPrefix = "/landscape/"
	case "9:16":
		aspectRatioPrefix = "/portrait/"
	default:
		aspectRatioPrefix = "/other/"
	}

	proccessedpath, err := processVideoForFastStart(tmp.Name())
	if err != nil {
		fmt.Printf("error while proccessing video for fast start: %v\n", err)
		respondWithError(w, 500, "Internal error", nil)
		return
	}

	videoForUpload, err := os.Open(proccessedpath)
	if err != nil {
		fmt.Printf("error while opening proccesed video file: %v\n", err)
		respondWithError(w, 500, "Internal error", nil)
		return
	}
	defer os.Remove(proccessedpath)
	defer videoForUpload.Close()

	randomBytes := make([]byte, 32)
	_, err = rand.Read(randomBytes)
	if err != nil {
		fmt.Printf("error while generating random name: %v\n", err)
		respondWithError(w, 500, "Something went wrong", nil)
		return
	}

	randomVideoID := aspectRatioPrefix + base64.RawURLEncoding.EncodeToString(randomBytes) + ".mp4"

	params := s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(randomVideoID),
		Body:        videoForUpload,
		ContentType: aws.String(mediaType),
	}
	_, err = cfg.s3Client.PutObject(context.Background(), &params)
	if err != nil {
		fmt.Printf("error while uploading object: %v\n", err)
		respondWithError(w, 500, "Something went wrong", nil)
		return
	}

	videoURL := cfg.createVideoURL(randomVideoID)
	video.VideoURL = &videoURL
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		fmt.Printf("error while updating database: %v\n", err)
		respondWithError(w, 500, "Something went wrong", nil)
		return
	}

	respondWithJSON(w, http.StatusOK, struct{}{})
}

func processVideoForFastStart(filepath string) (string, error) {
	outputfile := filepath + ".processing"
	cmd := exec.Command("ffmpeg", "-i", filepath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputfile)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("ffmpeg error: %w, stderr: %s", err, stderr.String())
	}

	return outputfile, nil
}
