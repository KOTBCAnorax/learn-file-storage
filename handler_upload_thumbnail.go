package main

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
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

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory = 10 << 20

	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		log.Printf("error: %v", err)
		return
	}

	file, fileHeader, err := r.FormFile("thumbnail")
	if err != nil {
		log.Printf("error: %v", err)
		return
	}

	fileType := fileHeader.Header.Get("Content-type")

	fileData, err := io.ReadAll(file)
	if err != nil {
		log.Printf("error: %v", err)
		return
	}

	metaData, err := cfg.db.GetVideo(videoID)
	if err != nil {
		log.Printf("error: %v", err)
		return
	}

	if metaData.UserID != userID {
		log.Printf("error: unauthorized")
		respondWithError(w, http.StatusUnauthorized, "unauthorized", nil)
	}

	newThumbnail := thumbnail{
		data:      fileData,
		mediaType: fileType,
	}

	videoThumbnails[videoID] = newThumbnail
	newThumbnailUrl := "http://localhost:8091/api/thumbnails/" + videoID.String()
	metaData.ThumbnailURL = &newThumbnailUrl

	cfg.db.UpdateVideo(metaData)

	respondWithJSON(w, http.StatusOK, metaData)
}
