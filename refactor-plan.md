### Phase 1: Foundation Preparation (Safe Changes)
1. **Define Core Interfaces and Structs**
   - Create `internal/core/types.go` with:
     ```go
     type Downloader interface {
         Validate(job *Job) error
         Prepare(job *Job) error 
         Execute(ctx context.Context) error
         Cleanup(job *Job) error
     }
     
     type Job struct {
         Type     string
         URL      string
         Output   string
         Config   map[string]interface{}
         Metadata map[string]string
     }
     ```

2. **Extract Shared Utilities**
   - Move all HTTP client logic to `internal/net/http_client.go`
   - Consolidate all progress/output management into `internal/output/manager.go`
   - Create `internal/utils/validation.go` for common validation logic

3. **Establish Configuration Hierarchy**
   - Create `config` package with:
     ```go
     type Config struct {
         Global   GlobalConfig
         Download map[string]interface{} // Downloader-specific
     }
     ```

### Phase 2: Command Layer Refactoring
1. **Restructure Cobra Commands**
   - Convert each download type to a subcommand in `cmd/`:
     ```
     cmd/
       root.go
       http/
         http.go
       s3/
         s3.go 
       batch/
         batch.go
     ```

2. **Implement Command Factories**
   - Create command builders in `cmd/factories.go`:
     ```go
     func NewHTTPCommand() *cobra.Command {
         return &cobra.Command{
             Use: "http [url]",
             RunE: func(cmd *cobra.Command, args []string) error {
                 job := core.NewJob("http", args[0])
                 // Parse flags into job.Config
                 return scheduler.Schedule(job)
             },
         }
     }
     ```

### Phase 3: Downloader Interface Implementation
1. **Convert Existing Downloaders**
   - For each downloader (HTTP, S3, etc.):
     - Create `downloaders/[type]/downloader.go`
     - Implement the `Downloader` interface
     - Example for HTTP:
       ```go
       type HTTPDownloader struct {
           client *http.Client
       }
       
       func (d *HTTPDownloader) Execute(ctx context.Context) error {
           // Existing download logic
       }
       ```

2. **Create Registration System**
   - In `internal/core/registry.go`:
     ```go
     var downloaders = make(map[string]Downloader)
     
     func RegisterDownloader(name string, d Downloader) {
         downloaders[name] = d
     }
     
     func GetDownloader(name string) (Downloader, error) {
         // ...
     }
     ```

### Phase 4: Scheduler Implementation
1. **Build Scheduler Core**
   - Create `internal/scheduler/scheduler.go`:
     ```go
     type Scheduler struct {
         workerPool chan struct{}
         jobQueue   chan *Job
     }
     
     func (s *Scheduler) Schedule(job *Job) error {
         // Add to queue
     }
     
     func (s *Scheduler) worker() {
         // Process jobs
     }
     ```

2. **Implement Concurrency Controls**
   - Add connection pooling and rate limiting

### Phase 5: Gradual Migration
1. **Phase 1 - HTTP Downloader**
   - Convert HTTP downloader first while keeping old code
   - Add feature flag to switch between implementations

2. **Phase 2 - Batch Processing**
   - Refactor YAML processing to use new Job structs
   - Update batch command to use scheduler

3. **Phase 3 - Remaining Downloaders**
   - Convert each downloader one by one
   - S3 → YouTube → Git → etc.

### Phase 6: Output System Upgrade
1. **Abstract Output Management**
   - Create `OutputHandler` interface:
     ```go
     type OutputHandler interface {
         Progress(job *Job, bytes int64)
         Complete(job *Job)
         Error(job *Job, err error)
     }
     ```

2. **Implement Multiple Formats**
   - Build JSON, text, and interactive output handlers

### Phase 7: Final Consolidation
1. **Remove Legacy Code**
   - Delete old implementations after full migration
   - Update documentation

2. **Add Extension Points**
   - Create plugin system for new downloaders
   - Add hook system for custom processing

### Recommended Refactoring Order:
1. Start with HTTP downloader (most complex)
2. Move to batch processing
3. Convert S3 downloader
4. Handle YouTube/Git special cases
5. Finally Google Drive auth flow

### Key Safety Mechanisms:
1. **Feature Flags**
   ```go
   const useNewDownloader = true
   
   func Download() {
       if useNewDownloader {
           // new
       } else {
           // old
       }
   }
   ```

2. **Integration Tests**
   - Maintain existing download tests
   - Add golden file tests for output formats

3. **Parallel Development**
   - Develop new system alongside old
   - Use branch-by-abstraction pattern

### Example First Step (HTTP Downloader):
1. Create `downloaders/http/structs.go`:
   ```go
   package http
   
   type Config struct {
       Connections int
       Headers     map[string]string
   }
   
   type Downloader struct {
       cfg    *Config
       client *http.Client
   }
   ```

2. Implement interface methods in `downloaders/http/downloader.go`

3. Register in `cmd/http/http.go`:
   ```go
   func init() {
       core.RegisterDownloader("http", http.NewDownloader())
   }
   ```

This incremental approach allows you to:
- Maintain working functionality throughout
- Test each component in isolation
- Roll back changes easily if needed
- Measure performance impact at each step


---

Let me break down **Phase 1: Foundation Preparation** in complete detail with concrete code examples and implementation steps. This phase is critical as it establishes the architectural foundation without breaking existing functionality.

---

### **Phase 1 Goals**
1. Define core interfaces and data structures
2. Extract shared utilities into reusable packages
3. Create a configuration system
4. Do all this *without modifying existing downloader implementations yet*

---

### **Step 1: Core Types and Interfaces**
Create `internal/core/types.go`:
```go
package core

import "context"

// Downloader interface that all download types will implement
type Downloader interface {
    Validate(job *Job) error           // Validate config/inputs
    Prepare(job *Job) error            // Pre-download setup (auth, temp files)
    Execute(ctx context.Context) error // Main download logic
    Cleanup(job *Job) error            // Post-download cleanup
}

// Job represents a single download task
type Job struct {
    ID       string                 // Unique identifier
    Type     string                 // "http", "s3", "youtube"
    URL      string                 // Resource location
    Output   string                 // Local output path
    Config   map[string]interface{} // Type-specific parameters
    Metadata map[string]string      // Context/tracing info
    Status   JobStatus              // Tracking state
}

type JobStatus struct {
    Progress int64   // Bytes downloaded
    Speed    float64 // Bytes/sec
    Error    error   // Failure reason
}

// NewJob creates a properly initialized Job
func NewJob(jobType, url string) *Job {
    return &Job{
        ID:       generateID(),
        Type:     jobType,
        URL:      url,
        Config:   make(map[string]interface{}),
        Metadata: make(map[string]string),
    }
}
```

---

### **Step 2: HTTP Utilities Refactor**
Create `internal/net/http_client.go`:
```go
package net

import (
    "time"
    "crypto/tls"
    "net"
    "net/http"
)

type ClientConfig struct {
    Timeout       time.Duration
    TLSConfig     *tls.Config
    ProxyURL      string
    MaxIdleConns  int
    // ... other settings
}

func NewClient(cfg ClientConfig) *http.Client {
    transport := &http.Transport{
        DialContext: (&net.Dialer{
            Timeout:   cfg.Timeout,
            KeepAlive: 30 * time.Second,
        }).DialContext,
        MaxIdleConns:        cfg.MaxIdleConns,
        IdleConnTimeout:     90 * time.Second,
        TLSHandshakeTimeout: 10 * time.Second,
        TLSClientConfig:     cfg.TLSConfig,
    }

    return &http.Client{
        Transport: transport,
        Timeout:   cfg.Timeout,
    }
}
```

---

### **Step 3: Output Management**
Create `internal/output/manager.go`:
```go
package output

import "github.com/tanq16/danzo/internal/core"

type Manager struct {
    // ... existing output manager fields
}

func (m *Manager) TrackJob(job *core.Job) {
    // Connect job progress to existing output system
    go func() {
        for {
            select {
            case <-job.Ctx.Done():
                return
            default:
                m.UpdateProgress(job.ID, job.Status.Progress, job.Status.Speed)
            }
        }
    }()
}

// Unified progress handling
func (m *Manager) UpdateProgress(jobID string, bytes int64, speed float64) {
    // ... existing progress bar logic
}
```

---

### **Step 4: Configuration System**
Create `config/config.go`:
```go
package config

import (
    "os"
    "gopkg.in/yaml.v3"
)

type GlobalConfig struct {
    Workers    int           `yaml:"workers"`
    Timeout    time.Duration `yaml:"timeout"`
    UserAgent  string        `yaml:"user_agent"`
}

func Load(path string) (*GlobalConfig, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    var cfg struct {
        Global GlobalConfig `yaml:"global"`
        // Other sections...
    }

    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, err
    }

    return &cfg.Global, nil
}
```

---

### **Step 5: Validation Utilities**
Create `internal/utils/validation.go`:
```go
package utils

import (
    "net/url"
    "path/filepath"
)

func ValidateURL(rawURL string) error {
    _, err := url.ParseRequestURI(rawURL)
    return err
}

func ValidateOutputPath(path string) error {
    if !filepath.IsAbs(path) {
        return filepath.Abs(path) // Convert to absolute path
    }
    return nil
}
```

---

### **Integration with Existing Code**
1. **Modify `cmd/root.go`** to use new types:
```go
// In runHTTP():
job := core.NewJob("http", url)
job.Config["output"] = outputPath
job.Config["connections"] = connections

// Instead of direct download call:
downloader, err := core.GetDownloader("http")
if err := downloader.Execute(job); err != nil {
    // Handle error
}
```

2. **Update output manager calls** to use `Job` struct:
```go
// Replace utils.PrintProgress() with:
outputManager.TrackJob(job)
```

---

### **Safety Mechanisms**
1. **Feature Flags** in `internal/core/features.go`:
```go
package core

var (
    EnableNewHTTP = false
    EnableNewS3   = false
    // ... other feature flags
)
```

2. **Adapter Pattern** for gradual migration:
```go
// LegacyHTTPAdapter wraps old HTTP downloader
type LegacyHTTPAdapter struct{}

func (a *LegacyHTTPAdapter) Execute(job *Job) error {
    // Call existing danzohttp.PerformMultiDownload()
    // while mapping Job struct to old parameters
}
```

---

### **Verification Steps**
1. **Test Existing Functionality**:
```bash
# Verify HTTP downloads still work
go test ./downloaders/http/... -v

# Verify batch processing
danzo -l download.yaml
```

2. **Benchmark Performance**:
```go
func BenchmarkNewHTTP(b *testing.B) {
    job := core.NewJob("http", testURL)
    // ... run benchmarks
}
```

---

### **Key Benefits of This Phase**
1. **No Breaking Changes**: Old code continues to work
2. **Clear Interfaces**: Defined contracts for future development
3. **Centralized Utilities**: No duplicated HTTP/client logic
4. **Preparation for Phase 2**: Command layer will build on these types

Would you like me to:
1. Provide more detailed code for any specific component?
2. Show how this integrates with a particular downloader (e.g., S3)?
3. Explain the testing strategy in more depth?

---

Let's dive deep into **Phase 2: Command Layer Refactoring**, where we'll restructure the CLI interface to be more maintainable and aligned with the new architecture. This phase focuses on the Cobra command structure while maintaining backward compatibility.

---

### **Phase 2 Goals**
1. Convert monolithic root command to modular subcommands
2. Standardize flag handling across download types
3. Implement command factories for consistent construction
4. Prepare for job scheduling integration

---

### **Step 1: Command Directory Structure**
```
cmd/
  root.go          # Base command
  common/          # Shared flags and helpers
    flags.go
    validation.go
  http/
    http.go       # HTTP subcommand
  s3/
    s3.go         # S3 subcommand
  batch/
    batch.go      # Batch processor
```

---

### **Step 2: Shared Flag System**
`cmd/common/flags.go`:
```go
package common

import (
    "github.com/spf13/cobra"
    "time"
)

// Flags used across multiple commands
type GlobalFlags struct {
    Output      string
    Connections int
    Timeout     time.Duration
}

// AddSharedFlags injects common flags to any command
func AddSharedFlags(cmd *cobra.Command, flags *GlobalFlags) {
    cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output file path")
    cmd.Flags().IntVarP(&flags.Connections, "connections", "c", 8, "Number of connections")
    cmd.Flags().DurationVarP(&flags.Timeout, "timeout", "t", 3*time.Minute, "Connection timeout")
}

// ValidateSharedFlags performs common validation
func ValidateSharedFlags(flags GlobalFlags) error {
    if flags.Connections < 1 || flags.Connections > 64 {
        return errors.New("connections must be between 1-64")
    }
    // ... other validations
}
```

---

### **Step 3: HTTP Subcommand Implementation**
`cmd/http/http.go`:
```go
package httpcmd

import (
    "github.com/spf13/cobra"
    "github.com/tanq16/danzo/internal/core"
    "github.com/tanq16/danzo/cmd/common"
)

type httpFlags struct {
    common.GlobalFlags
    Headers []string
}

func NewCommand() *cobra.Command {
    var flags httpFlags
    
    cmd := &cobra.Command{
        Use:   "http [URL]",
        Short: "Download files via HTTP",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            if err := common.ValidateSharedFlags(flags.GlobalFlags); err != nil {
                return err
            }

            job := core.NewJob("http", args[0])
            job.Config = map[string]interface{}{
                "output":      flags.Output,
                "connections": flags.Connections,
                "headers":     parseHeaders(flags.Headers),
            }

            return scheduleJob(job)
        },
    }

    common.AddSharedFlags(cmd, &flags.GlobalFlags)
    cmd.Flags().StringArrayVarP(&flags.Headers, "header", "H", nil, "Custom headers")
    
    return cmd
}

func parseHeaders(raw []string) map[string]string {
    // ... existing header parsing logic
}
```

---

### **Step 4: Batch Command Refactor**
`cmd/batch/batch.go`:
```go
package batchcmd

import (
    "github.com/spf13/cobra"
    "github.com/tanq16/danzo/internal/core"
    "gopkg.in/yaml.v3"
)

func NewCommand() *cobra.Command {
    var flags struct {
        File     string
        Workers  int
    }
    
    cmd := &cobra.Command{
        Use:   "batch -f FILE",
        Short: "Process batch downloads from YAML",
        RunE: func(cmd *cobra.Command, args []string) error {
            entries, err := parseBatchFile(flags.File)
            if err != nil {
                return err
            }

            jobs := make([]*core.Job, 0, len(entries))
            for _, entry := range entries {
                job := core.NewJob(entry.Type, entry.URL)
                job.Output = entry.OutputPath
                jobs = append(jobs, job)
            }

            return scheduleBatch(jobs, flags.Workers)
        },
    }

    cmd.Flags().StringVarP(&flags.File, "file", "f", "", "Batch file path")
    cmd.Flags().IntVarP(&flags.Workers, "workers", "w", 4, "Parallel workers")
    
    return cmd
}
```

---

### **Step 5: Root Command Restructuring**
`cmd/root.go`:
```go
package cmd

import (
    "github.com/spf13/cobra"
    "github.com/tanq16/danzo/cmd/batchcmd"
    "github.com/tanq16/danzo/cmd/httpcmd"
    // ... other subcommands
)

func NewRootCmd() *cobra.Command {
    rootCmd := &cobra.Command{
        Use:   "danzo",
        Short: "Multi-protocol download accelerator",
    }

    // Register subcommands
    rootCmd.AddCommand(
        httpcmd.NewCommand(),
        batchcmd.NewCommand(),
        // ... other commands
    )

    // Legacy flags for backward compatibility
    addLegacyFlags(rootCmd)
    
    return rootCmd
}

func addLegacyFlags(cmd *cobra.Command) {
    // Only add if no subcommand specified
    cmd.Flags().StringP("output", "o", "", "Legacy output flag")
    cmd.Flags().MarkHidden("output") // Hide from help
}
```

---

### **Step 6: Job Scheduling Bridge**
`cmd/scheduler.go` (temporary integration layer):
```go
package cmd

import (
    "context"
    "github.com/tanq16/danzo/internal/core"
    "github.com/tanq16/danzo/internal/scheduler"
)

// Temporary bridge until Phase 4 completion
func scheduleJob(job *core.Job) error {
    // Fallback to old implementation if feature flag disabled
    if !core.FeatureFlags.NewScheduler {
        return legacyDownload(job)
    }
    
    sched := scheduler.New()
    return sched.Schedule(context.Background(), job)
}

func scheduleBatch(jobs []*core.Job, workers int) error {
    // Similar dual-path implementation
}
```

---

### **Integration Strategy**
1. **Dual Command Registration**:
```go
// In httpcmd/http.go:
func init() {
    // Register both new and old commands
    rootCmd.AddCommand(NewCommand())
    rootCmd.AddCommand(NewLegacyCommand()) 
}
```

2. **Feature Flags**:
```go
// In cmd/http/http.go:
if core.FeatureFlags.NewHTTPCommand {
    return NewCommand()
}
return NewLegacyCommand()
```

---

### **Verification Process**
1. **Test All Command Paths**:
```bash
# New style
danzo http https://example.com/file.zip -o out.zip

# Old style (should still work)
danzo --legacy https://example.com/file.zip -o out.zip
```

2. **Validation Checks**:
```go
func TestCommandFlags(t *testing.T) {
    tests := []struct {
        args     []string
        valid    bool
    }{
        {[]string{"http", "https://valid.url"}, true},
        {[]string{"http", "invalid-url"}, false},
    }
    
    for _, tt := range tests {
        cmd := NewRootCmd()
        cmd.SetArgs(tt.args)
        err := cmd.Execute()
        // Assertions...
    }
}
```

---

### **Key Safety Mechanisms**
1. **Legacy Fallback**:
```go
func runE(cmd *cobra.Command, args []string) error {
    if legacyMode(cmd) {
        return runLegacy(cmd, args)
    }
    // New logic...
}
```

2. **Flag Migration**:
```go
// Convert legacy flags to new format
func migrateFlags(cmd *cobra.Command, job *core.Job) {
    if cmd.Flag("output").Changed {
        job.Output = cmd.Flag("output").Value.String()
    }
}
```

3. **Deprecation Warnings**:
```go
if cmd.Flag("legacy").Changed {
    log.Println("Warning: Legacy mode will be removed in v2.0")
}
```

---

### **Phase Completion Checklist**
1. [ ] All download types have dedicated subcommands
2. [ ] Shared flag validation working
3. [ ] Batch processing uses new Job structs
4. [ ] Legacy commands still functional
5. [ ] Test coverage for new command paths

Would you like me to:
1. Show the S3 subcommand implementation in detail?
2. Explain how to handle the YouTube command's special cases?
3. Demonstrate the batch YAML parsing changes?
