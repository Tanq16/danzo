package internal

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/tanq16/danzo/utils"
)

type ProgressInfo struct {
	OutputPath    string
	TotalSize     int64
	Downloaded    int64
	Speed         float64
	AvgSpeed      float64
	ETA           string
	Completed     bool
	CompletedSize int64
	StartTime     time.Time
}

type ProgressManager struct {
	progressMap     map[string]*ProgressInfo
	mutex           sync.RWMutex
	doneCh          chan struct{}
	lastUpdateTimes map[string]time.Time
	lastDownloaded  map[string]int64
	numLines        int
}

func NewProgressManager() *ProgressManager {
	return &ProgressManager{
		progressMap:     make(map[string]*ProgressInfo),
		doneCh:          make(chan struct{}),
		lastUpdateTimes: make(map[string]time.Time),
		lastDownloaded:  make(map[string]int64),
	}
}

func (pm *ProgressManager) Register(outputPath string, totalSize int64) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	pm.progressMap[outputPath] = &ProgressInfo{
		OutputPath: outputPath,
		TotalSize:  totalSize,
		StartTime:  time.Now(),
	}
	pm.lastUpdateTimes[outputPath] = time.Now()
	pm.lastDownloaded[outputPath] = 0
}

func (pm *ProgressManager) Update(outputPath string, bytesDownloaded int64) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	if info, exists := pm.progressMap[outputPath]; exists {
		info.Downloaded += bytesDownloaded
	}
}

func (pm *ProgressManager) Complete(outputPath string, totalDownloaded int64) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	if info, exists := pm.progressMap[outputPath]; exists {
		info.Completed = true
		info.CompletedSize = totalDownloaded
	}
	log.Debug().Str("file", outputPath).Int64("totalDownloaded", totalDownloaded).Msg("COMPLETE CALLED")
}

func (pm *ProgressManager) StartDisplay() {
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				pm.updateDisplay()
			case <-pm.doneCh:
				return
			}
		}
	}()
}

func (pm *ProgressManager) updateDisplay() {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	if pm.numLines > 0 {
		fmt.Printf("\033[%dA\033[J", pm.numLines)
	}
	var keys []string
	numActive := 0
	for outputPath, info := range pm.progressMap {
		keys = append(keys, outputPath)
		if !info.Completed {
			numActive++
		}
	}
	sort.Strings(keys)
	if numActive == 0 && len(pm.progressMap) > 0 {
		fmt.Println("All downloads completed.")
		pm.numLines = 1
		return
	}
	pm.numLines = 0

	for _, outputPath := range keys {
		info := pm.progressMap[outputPath]
		if info.Completed {
			continue
		}
		fileName := outputPath
		if len(fileName) > 25 {
			fileName = "..." + fileName[len(fileName)-22:]
		}
		now := time.Now()
		lastTime, exists := pm.lastUpdateTimes[outputPath]
		if !exists {
			lastTime = info.StartTime
		}
		timeDiff := now.Sub(lastTime).Seconds()
		byteDiff := info.Downloaded - pm.lastDownloaded[outputPath]
		speed := float64(0)
		if timeDiff > 0 {
			speed = float64(byteDiff) / timeDiff / 1024 / 1024 // MB/s
			pm.lastUpdateTimes[outputPath] = now
			pm.lastDownloaded[outputPath] = info.Downloaded
		}
		info.Speed = speed
		eta := "calculating..."
		if info.Speed > 0 && info.TotalSize > 0 {
			etaSeconds := int64(float64(info.TotalSize-info.Downloaded) / (info.Speed * 1024 * 1024))
			if etaSeconds < 60 {
				eta = fmt.Sprintf("%ds", etaSeconds)
			} else if etaSeconds < 3600 {
				eta = fmt.Sprintf("%dm %ds", etaSeconds/60, etaSeconds%60)
			} else {
				eta = fmt.Sprintf("%dh %dm", etaSeconds/3600, (etaSeconds%3600)/60)
			}
		}
		info.ETA = eta

		// progress bar
		progressWidth := 30
		var progressBar string
		if info.TotalSize > 0 {
			percent := float64(info.Downloaded) / float64(info.TotalSize)
			filledWidth := int(percent * float64(progressWidth))
			progressBar = "["
			progressBar += strings.Repeat("=", filledWidth)
			if filledWidth < progressWidth {
				progressBar += ">"
				progressBar += strings.Repeat(" ", progressWidth-filledWidth-1)
			}
			progressBar += "]"
			fmt.Printf("%s: %s %.1f%% %s/%s %.2f MB/s ETA: %s\n", fileName, progressBar, percent*100, utils.FormatBytes(uint64(info.Downloaded)), utils.FormatBytes(uint64(info.TotalSize)), info.Speed, eta)
		} else {
			// total size unknown
			progressBar = "[" + strings.Repeat("=", 15) + ">" + strings.Repeat(" ", 14) + "]"
			fmt.Printf("%s: %s %s %.2f MB/s\n", fileName, progressBar, utils.FormatBytes(uint64(info.Downloaded)), info.Speed)
		}
		pm.numLines++
	}
}

func (pm *ProgressManager) ShowSummary() {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	fmt.Println()
	fmt.Println("Task Summary")
	fmt.Println("============")
	totalSize := int64(0)
	earliestTime := float64(0)

	for _, info := range pm.progressMap {
		elapsed := time.Since(info.StartTime).Seconds()
		if earliestTime == 0 || elapsed > earliestTime {
			earliestTime = elapsed
		}
		totalSize += info.CompletedSize
		status := "Completed"
		if !info.Completed {
			status = "Incomplete"
		}
		fmt.Printf("Status: %s,  Size: %s,  File: %s\n", status, utils.FormatBytes(uint64(info.CompletedSize)), info.OutputPath)
	}
	fmt.Println()
	overallSpeed := float64(totalSize) / earliestTime / 1024 / 1024
	log.Info().Str("Total Data", utils.FormatBytes(uint64(totalSize))).Str("Overall Speed", fmt.Sprintf("%.2f MB/s", overallSpeed)).Str("Time Elapsed", fmt.Sprintf("%.2fs", earliestTime)).Msg("Summary")
}

func (pm *ProgressManager) Stop() {
	close(pm.doneCh)
}
