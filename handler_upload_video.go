package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"

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

	video, err := cfg.db.GetVideo(videoID)

	if err != nil {
		respondWithError(w, http.StatusBadRequest, "couldn't find video", err)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "unauthorized", err)
		return
	}

	uploaderLimit := int64(1 << 30)
	closeReader := http.MaxBytesReader(w, r.Body, uploaderLimit)

	multipartFile, multipartHeader, err := r.FormFile("video")
	
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to parse file", err)
		return
	}

	defer multipartFile.Close()
	defer closeReader.Close()

	contentType := multipartHeader.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)

	if err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid media type", err)
		return
	}

	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "invalid media type", err)
		return
	}

	tmpFile, err := os.CreateTemp("", "tubely-upload.mp4")

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to create temp file", err)
		return
	}


	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, multipartFile)

	if err != nil {
		respondWithError(w, http.StatusBadRequest, "failed file copy", err)
		return
	}

	tmpFile.Seek(0, io.SeekStart)

	randomBytes := make([]byte, 32)
	rand.Read(randomBytes)
	randomName := hex.EncodeToString(randomBytes)
	fileName := randomName + ".mp4"

	aspectRatio, err := getVideoAspectRatio(tmpFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to get video aspect ratio", err)
		return
	}

  var pathPrefix string

  if aspectRatio == "16:9" {
		pathPrefix = "landscape"
	} else if aspectRatio == "9:16" {
		pathPrefix = "portrait"
	} else {
		pathPrefix = "other"
	}

	fileName = fmt.Sprintf("%s/%s", pathPrefix, fileName)

	outputFilePath, err := procesVideoForFastStart(tmpFile.Name())
	
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to process video", err)
		return 
	}

	outputFile, err := os.ReadFile(outputFilePath)

	defer os.Remove(outputFilePath)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError,  "failed to read output file", err)
		return
	}

	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket: &cfg.s3Bucket,
		Key: &fileName,
		Body: bytes.NewReader(outputFile),
		ContentType: &mediaType,
	})

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to upload file", err)
		return
	}
	
	videoUrl := fmt.Sprintf("https://%s/%s", cfg.s3CfDistribution, fileName)

	video.VideoURL = &videoUrl

	err = cfg.db.UpdateVideo(video)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to update video", err)
		return
	}
}

func getVideoAspectRatio(filePath string) (string, error) {
	ffprobeCmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	output, err := ffprobeCmd.Output()

	if err != nil {
		return "", err
	}
	
	var result map[string]interface{}
	err = json.Unmarshal(output, &result)
	if err != nil {
		return "", err
	}

	stream := result["streams"].([]interface{})[0].(map[string]interface{})
	width := stream["width"].(float64)
	height := stream["height"].(float64)

	aspectRatio :=  uint64(width /	 height)

	switch aspectRatio {
	case uint64(16/9):
		return "16:9", nil
	case 9/16:
		return "9:16", nil
	default:
		return "other", nil
	}
}

func procesVideoForFastStart (filePath string) (string, error) {
	outputFilePath := fmt.Sprintf("%s.processing", filePath)

	processVideoCommand := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputFilePath)
	
	err := processVideoCommand.Run()

	if err != nil {
		return "", fmt.Errorf("Error while process video: %w")
	}

	return outputFilePath, nil
}
