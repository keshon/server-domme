package media

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
)

var (
	recentHistory   = []string{}
	historyLimit    = 20       // how many past items to remember
	recencyDecay    = 0.5      // higher = stronger penalty for recent items
	recentHistoryMu sync.Mutex // thread safety for concurrent commands
)

// pickRandomFile returns a random media file
func pickRandomFile(folder string) (string, error) {

	if folder == "" {
		folder = "./assets/media"
	}

	if _, err := os.Stat(folder); os.IsNotExist(err) {
		return "", fmt.Errorf("media folder does not exist")
	}

	files := []string{}
	err := filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			ext := filepath.Ext(info.Name())
			switch ext {
			case ".mp4", ".webm", ".mov", ".gif", ".jpg", ".png":
				files = append(files, path)
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no media files found")
	}

	// return files[rand.Intn(len(files))], nil
	return pickWeightedRandomFile(files), nil
}

// pickWeightedRandomFile returns a random file from list with bias against recent ones
func pickWeightedRandomFile(files []string) string {
	recentHistoryMu.Lock()
	defer recentHistoryMu.Unlock()

	if len(files) == 0 {
		return ""
	}
	if len(files) == 1 {
		updateHistory(files[0])
		return files[0]
	}

	weights := make([]float64, len(files))
	for i, file := range files {
		recencyIndex := findInHistory(file)
		if recencyIndex == -1 {
			weights[i] = 1.0
		} else {
			positionFromEnd := len(recentHistory) - recencyIndex - 1
			weights[i] = math.Exp(-recencyDecay * float64(positionFromEnd))
		}
	}

	total := 0.0
	for _, w := range weights {
		total += w
	}

	r := rand.Float64() * total
	acc := 0.0
	for i, w := range weights {
		acc += w
		if r <= acc {
			updateHistory(files[i])
			return files[i]
		}
	}

	updateHistory(files[len(files)-1])
	return files[len(files)-1]
}

func findInHistory(file string) int {
	for i, f := range recentHistory {
		if f == file {
			return i
		}
	}
	return -1
}

func updateHistory(file string) {
	recentHistory = append(recentHistory, file)
	if len(recentHistory) > historyLimit {
		recentHistory = recentHistory[len(recentHistory)-historyLimit:]
	}
}
