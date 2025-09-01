package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
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

	randomBytes := make([]byte, 32)
	rand.Read(randomBytes)
	randomName := base64.RawURLEncoding.EncodeToString(randomBytes);

	extension := strings.Split(contentType, "/")[1]
	fileName := randomName + "." + extension
	filePath := cfg.assetsRoot + "/" + fileName

	diskFile, err := os.Create(filePath)

	if err != nil {
		respondWithError(w, 500, "failed to save file", err)
		return
	}

	mediaType, _, err := mime.ParseMediaType(contentType)

	if err != nil {
		respondWithError(w, 500, "failed to parse mediatype", err)
		return
	}

	if mediaType != "image/png" && mediaType != "image/jped" {
		respondWithError(w, 500, "unsupported media type", errors.New("unsupported media type"))
		return
	}

	bytesReader := bytes.NewReader(fileBytes)
	written, err := io.Copy(diskFile, bytesReader)

	if err != nil || written == 0 {
		respondWithError(w, 500, "failed to save file", err)
		return
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
