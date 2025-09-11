package main

import (
	"net/http"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	uploaderLimit := int64(1 << 30)
	reader :=http.MaxBytesReader(w, r.Body, uploaderLimit)

}
