package heartbeat

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

type Task struct {
	Description string
	Frequency   time.Duration
}

// Parser reads HEARTBEAT.md and returns a list of periodic tasks.
func Parser(path string) ([]Task, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open heartbeat file: %w", err)
	}
	defer file.Close()

	var tasks []Task
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "- [ ]") || strings.HasPrefix(line, "- [x]") {
			// Basic parser: look for task lines
			parts := strings.SplitN(line, " ", 3)
			if len(parts) < 3 {
				continue
			}
			desc := parts[2]
			// Default frequency for now: 30 minutes
			tasks = append(tasks, Task{
				Description: desc,
				Frequency:   30 * time.Minute,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan heartbeat file: %w", err)
	}

	return tasks, nil
}
