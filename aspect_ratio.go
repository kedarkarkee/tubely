package main

import (
	"bytes"
	"encoding/json"
	"math"
	"os/exec"
)

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)

	buffer := bytes.NewBuffer([]byte{})
	cmd.Stdout = buffer
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	jsonStruct := &struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}{}

	err = json.Unmarshal(buffer.Bytes(), jsonStruct)

	if err != nil {
		return "", err
	}
	streamInfo := jsonStruct.Streams[0]

	return getAspectRatio(streamInfo.Width, streamInfo.Height), nil
}

func getAspectRatio(width, height int) string {
	if width == 0 || height == 0 {
		return "other"
	}

	ratio := float64(width) / float64(height)
	const tolerance = 0.02 // ~2%

	switch {
	case math.Abs(ratio-(16.0/9.0)) < tolerance:
		return "16:9"
	case math.Abs(ratio-(9.0/16.0)) < tolerance:
		return "9:16"
	default:
		return "other"
	}
}
