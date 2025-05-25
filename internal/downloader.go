package internal

				case "youtube":
					close(progressCh) // Note needed for youtube downloads
					processedURL, format, dType, output, err := danzoyoutube.ProcessURL(entry.URL)
					if err != nil {
						outputMgr.ReportError(entryFunctionId, fmt.Errorf("error processing YouTube URL %s: %v", entry.OutputPath, err))
						outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Error processing YouTube URL %s", entry.OutputPath))
						continue
					}
					if config.OutputPath == "" {
						config.OutputPath = output
						entry.OutputPath = output
					} else {
						existingFile, _ := os.Stat(config.OutputPath)
						if existingFile != nil {
							if existingFile.Size() > 0 {
								continue
							}
							config.OutputPath = utils.RenewOutputPath(config.OutputPath)
							entry.OutputPath = config.OutputPath
						} else {
							entry.OutputPath = config.OutputPath
						}
					}
					outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Downloading %s", entry.OutputPath))
					entry.URL = processedURL
					streamCh := make(chan []string, 7)

					// Goroutine to forward streaming output to the manager
					var streamWg sync.WaitGroup
					streamWg.Add(1)
					go func(outputPath string, streamCh <-chan []string) {
						defer streamWg.Done()
						for streamOutput := range streamCh {
							outputMgr.UpdateStreamOutput(entryFunctionId, streamOutput)
						}
					}(entry.OutputPath, streamCh)

					err = danzoyoutube.DownloadYouTubeVideo(entry.URL, entry.OutputPath, format, dType, streamCh)
					close(streamCh)
					if err != nil {
						outputMgr.ReportError(entryFunctionId, fmt.Errorf("error downloading %s: %v", entry.OutputPath, err))
						outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Error downloading %s", entry.OutputPath))
					} else {
						outputMgr.Complete(entryFunctionId, fmt.Sprintf("Download completed for %s", entry.OutputPath))
					}
					streamWg.Wait()
