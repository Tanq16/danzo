package danzohttp

// func PerformMultiDownload(config utils.DownloadConfig, client *utils.DanzoHTTPClient, fileSize int64, progressCh chan<- int64) error {
// 	job := utils.DownloadJob{
// 		Config:    config,
// 		FileSize:  fileSize,
// 		StartTime: time.Now(),
// 	}

// 	tempDir := filepath.Join(filepath.Dir(config.OutputPath), ".danzo-temp")
// 	if err := os.MkdirAll(tempDir, 0755); err != nil {
// 		return fmt.Errorf("error creating temp directory: %v", err)
// 	}

// 	// Setup chunks
// 	chunkSize := fileSize / int64(config.Connections)
// 	for i := 0; i < config.Connections; i++ {
// 		startByte := int64(i) * chunkSize
// 		endByte := startByte + chunkSize - 1
// 		if i == config.Connections-1 {
// 			endByte = fileSize - 1
// 		}

// 		job.Chunks = append(job.Chunks, utils.DownloadChunk{
// 			ID:        i,
// 			StartByte: startByte,
// 			EndByte:   endByte,
// 		})
// 	}

// 	// Start downloads
// 	var wg sync.WaitGroup
// 	mutex := &sync.Mutex{}

// 	for i := range job.Chunks {
// 		wg.Add(1)
// 		go chunkedDownload(&job, &job.Chunks[i], client, &wg, progressCh, mutex)
// 	}

// 	wg.Wait()

// 	// Check completion
// 	for _, chunk := range job.Chunks {
// 		if !chunk.Completed {
// 			return fmt.Errorf("chunk %d failed to complete", chunk.ID)
// 		}
// 	}

// 	// Assemble file
// 	return assembleFile(job)
// }

// func assembleFile(job utils.DownloadJob) error {
// 	// Sort temp files by chunk ID
// 	sort.Slice(job.TempFiles, func(i, j int) bool {
// 		idI, _ := extractChunkID(job.TempFiles[i])
// 		idJ, _ := extractChunkID(job.TempFiles[j])
// 		return idI < idJ
// 	})

// 	// Create final file
// 	destFile, err := os.Create(job.Config.OutputPath)
// 	if err != nil {
// 		return err
// 	}
// 	defer destFile.Close()

// 	// Copy chunks
// 	var totalWritten int64
// 	for _, tempFilePath := range job.TempFiles {
// 		tempFile, err := os.Open(tempFilePath)
// 		if err != nil {
// 			return fmt.Errorf("error opening chunk: %v", err)
// 		}

// 		written, err := io.Copy(destFile, tempFile)
// 		tempFile.Close()
// 		if err != nil {
// 			return fmt.Errorf("error copying chunk: %v", err)
// 		}

// 		totalWritten += written
// 	}

// 	if totalWritten != job.FileSize {
// 		return fmt.Errorf("size mismatch: expected %d, got %d", job.FileSize, totalWritten)
// 	}

// 	// Cleanup
// 	for _, tempFilePath := range job.TempFiles {
// 		os.Remove(tempFilePath)
// 	}

// 	return nil
// }

// func extractChunkID(filename string) (int, error) {
// 	matches := utils.ChunkIDRegex.FindStringSubmatch(filename)
// 	if len(matches) < 2 {
// 		return -1, errors.New("could not extract chunk ID")
// 	}
// 	return strconv.Atoi(matches[1])
// }
