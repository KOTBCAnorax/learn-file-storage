package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"log"
	"mime"
	"net/http"
	"os"

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
		log.Printf("error: %v\n", err)
		respondWithError(w, 500, "Internal error", nil)
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	_, err = io.Copy(tmp, file)
	if err != nil {
		log.Printf("error: %v\n", err)
		respondWithError(w, 500, "Internal error", nil)
		return
	}

	tmp.Seek(0, io.SeekStart)

	randomBytes := make([]byte, 32)
	_, err = rand.Read(randomBytes)
	if err != nil {
		log.Printf("error: %v\n", err)
		respondWithError(w, 500, "Something went wrong", nil)
		return
	}

	randomVideoID := base64.RawURLEncoding.EncodeToString(randomBytes) + ".mp4"

	params := s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(randomVideoID),
		Body:        tmp,
		ContentType: aws.String(mediaType),
	}
	_, err = cfg.s3Client.PutObject(context.Background(), &params)
	if err != nil {
		log.Printf("error: %v\n", err)
		respondWithError(w, 500, "Something went wrong", nil)
		return
	}

	videoURL := cfg.createVideoURL(randomVideoID)
	video.VideoURL = &videoURL
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		log.Printf("error: %v\n", err)
		respondWithError(w, 500, "Something went wrong", nil)
		return
	}

	respondWithJSON(w, http.StatusOK, struct{}{})
}
