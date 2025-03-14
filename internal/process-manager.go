package internal

import (
	"fmt"
	"sync"
	"time"
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
	progressMap map[string]*ProgressInfo
	mutex       sync.RWMutex
	doneCh      chan struct{}
}

func NewProgressManager() *ProgressManager {
	return &ProgressManager{
		progressMap: make(map[string]*ProgressInfo),
		doneCh:      make(chan struct{}),
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
}

func (pm *ProgressManager) IsAllCompleted() bool {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	for _, info := range pm.progressMap {
		if !info.Completed {
			return false
		}
	}
	return len(pm.progressMap) > 0
}

func (pm *ProgressManager) StartDisplay() {
	go func() {
		log := GetLogger("progress-manager")
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		currentIndex := 0
		lastUpdateTimes := make(map[string]time.Time)
		lastDownloaded := make(map[string]int64)
		clearLine := func() {
			fmt.Printf("\r\033[K") // Clear the current line
		}

		for {
			select {
			case <-ticker.C:
				pm.mutex.RLock()
				paths := make([]string, 0, len(pm.progressMap))
				for path, info := range pm.progressMap {
					if !info.Completed {
						paths = append(paths, path)
					}
				}
				if len(paths) > 0 {
					if currentIndex >= len(paths) {
						currentIndex = 0
					}
					outputPath := paths[currentIndex]
					info := pm.progressMap[outputPath]
					now := time.Now()
					lastTime, exists := lastUpdateTimes[outputPath]
					if !exists {
						lastTime = info.StartTime
						lastDownloaded[outputPath] = 0
					}
					timeDiff := now.Sub(lastTime).Seconds()
					byteDiff := info.Downloaded - lastDownloaded[outputPath]
					if timeDiff > 0 {
						info.Speed = float64(byteDiff) / timeDiff / 1024 / 1024 // MB/s
						lastUpdateTimes[outputPath] = now
						lastDownloaded[outputPath] = info.Downloaded
					}
					elapsed := time.Since(info.StartTime).Seconds()
					if elapsed > 0 {
						info.AvgSpeed = float64(info.Downloaded) / elapsed / 1024 / 1024 // MB/s
					}
					if info.Speed > 0 {
						etaSeconds := int64(float64(info.TotalSize-info.Downloaded) / (info.Speed * 1024 * 1024)) // from MB/s to B/s
						if etaSeconds < 60 {
							info.ETA = fmt.Sprintf("%ds", etaSeconds)
						} else if etaSeconds < 3600 {
							info.ETA = fmt.Sprintf("%dm %ds", etaSeconds/60, etaSeconds%60)
						} else {
							info.ETA = fmt.Sprintf("%dh %dm", etaSeconds/3600, (etaSeconds%3600)/60)
						}
					} else {
						info.ETA = "calculating..."
					}

					// Display progress
					percent := float64(info.Downloaded) / float64(info.TotalSize) * 100
					clearLine()
					fmt.Printf("[%s] %.2f%% (%s/%s) Speed: %.2f MB/s ETA: %s", outputPath, percent, formatBytes(uint64(info.Downloaded)), formatBytes(uint64(info.TotalSize)), info.Speed, info.ETA)
					log.Debug().Str("file", outputPath).Float64("percent", percent).Str("downloaded", formatBytes(uint64(info.Downloaded))).Str("total", formatBytes(uint64(info.TotalSize))).Float64("speed_mbps", info.Speed).Str("eta", info.ETA).Msg("Download progress")
					currentIndex++
				} else if pm.IsAllCompleted() {
					clearLine()
					log.Info().Msg("All downloads completed!")
					pm.mutex.RUnlock()
					return
				}
				pm.mutex.RUnlock()

			case <-pm.doneCh:
				clearLine()
				return
			}
		}
	}()
}

func (pm *ProgressManager) ShowSummary() {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	fmt.Println("\nDownload Summary:")
	fmt.Println("----------------")
	for _, info := range pm.progressMap {
		elapsed := time.Since(info.StartTime).Seconds()
		avgSpeed := float64(0)
		if elapsed > 0 {
			avgSpeed = float64(info.CompletedSize) / elapsed / 1024 / 1024 // MB/s
		}
		status := "Completed"
		if !info.Completed {
			status = "Incomplete"
		}
		fmt.Printf("%s: %s, Size: %s, Avg Speed: %.2f MB/s, Time: %.1fs\n", info.OutputPath, status, formatBytes(uint64(info.CompletedSize)), avgSpeed, elapsed)
	}
}
