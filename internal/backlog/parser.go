package backlog

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

// Task represents a single backlog task (e.g. from autonomous-agent-readiness-backlog.md).
type Task struct {
	ID           string
	Name         string
	Priority     string
	Status       string
	Dependencies string
}

var priorityOrder = map[string]int{
	"CRITICAL": 0,
	"HIGH":     1,
	"MEDIUM":   2,
}

// ParseBacklogFile reads a markdown file with a task table and returns parsed tasks.
// Expects table format: | Task ID | Name | Priority | Status | Dependencies |
func ParseBacklogFile(path string) ([]Task, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var tasks []Task
	scanner := bufio.NewScanner(f)
	inTable := false
	sep := regexp.MustCompile(`^\|?\s*[-:]+\s*\|`)
	for scanner.Scan() {
		line := scanner.Text()
		if sep.MatchString(line) {
			inTable = true
			continue
		}
		if !inTable {
			if strings.HasPrefix(line, "|") && strings.Contains(line, "Task ID") {
				continue
			}
			if strings.HasPrefix(line, "|") && strings.Contains(line, "|") {
				inTable = true
			} else {
				continue
			}
		}
		if !strings.HasPrefix(line, "|") {
			break
		}
		parts := splitTableRow(line)
		if len(parts) < 5 {
			continue
		}
		if strings.TrimSpace(parts[3]) == "Status" {
			continue
		}
		tasks = append(tasks, Task{
			ID:           strings.TrimSpace(parts[0]),
			Name:         strings.TrimSpace(parts[1]),
			Priority:     strings.TrimSpace(parts[2]),
			Status:       strings.TrimSpace(parts[3]),
			Dependencies: strings.TrimSpace(parts[4]),
		})
	}
	return tasks, scanner.Err()
}

func splitTableRow(line string) []string {
	var parts []string
	for _, s := range strings.Split(line, "|") {
		parts = append(parts, strings.TrimSpace(s))
	}
	if len(parts) >= 2 && parts[0] == "" && parts[len(parts)-1] == "" {
		parts = parts[1 : len(parts)-1]
	}
	return parts
}

// FirstPending returns the first task with Status == "PENDING" by priority order (CRITICAL, HIGH, MEDIUM).
func FirstPending(tasks []Task) *Task {
	var best *Task
	bestOrder := 999
	for i := range tasks {
		if tasks[i].Status != "PENDING" {
			continue
		}
		order, ok := priorityOrder[tasks[i].Priority]
		if !ok {
			order = 99
		}
		if order < bestOrder {
			bestOrder = order
			best = &tasks[i]
		}
	}
	return best
}
