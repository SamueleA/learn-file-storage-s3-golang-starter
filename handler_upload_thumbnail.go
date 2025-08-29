package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

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
	r.ParseMultipartForm(maxMemory)

	file, header, err := r.FormFile("thumbnail")

	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}

	defer file.Close()

	contentType := header.Header.Get("Content-Type")

	if contentType == "" {
		respondWithError(w, http.StatusBadRequest, "Unable to parse file type", err)
		return
	}

	fileBytes, err := io.ReadAll(file)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to read file", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to find video metadata", err)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "unauthorized", err)
		return
	}

	extension := strings.Split(contentType, "/")[1]
	fileName := videoIDString + "." + extension
	filePath := cfg.assetsRoot + "/" + fileName

	diskFile, err := os.Create(filePath)

	if err != nil {
		respondWithError(w, 500, "failed to save file", err)
		return
	}

	bytesReader := bytes.NewReader(fileBytes)
	written, err := io.Copy(diskFile, bytesReader)

	if err != nil || written == 0 {
		respondWithError(w, 500, "failed to save file", err)
	}


	assetsRoot := cfg.assetsRoot[1:]

	thumbnailURL := fmt.Sprintf("http://localhost:%s%s/%s", cfg.port, assetsRoot, fileName)

	video.ThumbnailURL = &thumbnailURL

	err = cfg.db.UpdateVideo(video)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update video metadata", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
