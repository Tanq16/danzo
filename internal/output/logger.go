package output

import (
	"sync"
)

type Logger struct {
	mu        sync.Mutex
	fileName  string
	functions []FunctionLogger
}

type FunctionLogger struct {
	ID          int
	URL         string
	Status      string
	Message     string
	StreamLines []string
}
