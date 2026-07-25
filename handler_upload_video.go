package main

import (
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
	uploadLimitBytes := 1 << 30 //1GB
	http.MaxBytesReader(w, r.Body, int64(uploadLimitBytes))

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

	fmt.Println("uploading video", videoID, "by user", userID)

	video, err := cfg.db.GetVideo(videoID)

	if err != nil {
		respondWithError(w, http.StatusNotFound, "Video not found", err)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "User not authorized for this video", err)
		return
	}

	file, header, err := r.FormFile("video")

	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse video", err)
		return
	}

	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))

	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}

	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid video format", err)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Something went wrong", err)
		return
	}

	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, file)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldnot upload video 1", err)
		return
	}

	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldnot upload video 2", err)
		return
	}

	aspectRatio, err := getVideoAspectRatio(tempFile.Name())

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldnot upload video 3", err)
		return
	}

	if aspectRatio == "16:9" {
		aspectRatio = "landscape"
	} else if aspectRatio == "9:16" {
		aspectRatio = "portrait"
	}
	processedVideo, err := processVideoForFastStart(tempFile.Name())

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldnot upload video 4", err)
		return
	}

	defer os.Remove(processedVideo)

	processedVideoBytes, err := os.Open(processedVideo)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldnot upload video 5", err)
		return
	}

	defer processedVideoBytes.Close()

	assetKey := aspectRatio + "/" + getAssetPath(mediaType)
	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &assetKey,
		Body:        processedVideoBytes,
		ContentType: &mediaType,
	})

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldnot upload video to s3", err)
		return
	}

	videoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, assetKey)

	video.VideoURL = &videoURL

	err = cfg.db.UpdateVideo(video)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Video could not be uploaded", err)
		return
	}
	respondWithJSON(w, http.StatusOK, video)
}

func processVideoForFastStart(filePath string) (string, error) {
	tempFile := filePath + ".processing"
	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", tempFile)
	err := cmd.Run()

	if err != nil {
		return "", nil
	}
	return tempFile, nil
}
