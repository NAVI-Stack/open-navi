package schema

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// RiskLevel classifies the blast radius of a task.
type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

// TaskStatus is the lifecycle state machine for a task.
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusBlocked   TaskStatus = "blocked"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// TaskClass describes the intake contract for project-scoped work.
type TaskClass string

const (
	TaskClassReadOnly   TaskClass = "read_only"
	TaskClassPlanning   TaskClass = "planning"
	TaskClassMutative   TaskClass = "mutative"
	TaskClassSelfUpdate TaskClass = "self_update"
)

var validTaskClasses = map[TaskClass]struct{}{
	TaskClassReadOnly:   {},
	TaskClassPlanning:   {},
	TaskClassMutative:   {},
	TaskClassSelfUpdate: {},
}

func (c TaskClass) IsValid() bool {
	_, ok := validTaskClasses[c]
	return ok
}

func ParseTaskClass(raw string) (TaskClass, error) {
	normalized := TaskClass(strings.ToLower(strings.TrimSpace(raw)))
	if _, ok := validTaskClasses[normalized]; !ok {
		return "", fmt.Errorf("task: unknown task class %q", raw)
	}
	return normalized, nil
}

func (c TaskClass) RequiresWorkspaceBinding() bool {
	return c == TaskClassMutative || c == TaskClassSelfUpdate
}

// CostTier buckets API spending so the governor can apply tier-level caps
// before evaluating exact USD amounts.
type CostTier string

const (
	CostFree      CostTier = "free"
	CostCheap     CostTier = "cheap"
	CostModerate  CostTier = "moderate"
	CostExpensive CostTier = "expensive"
)

// DAGEdge declares a dependency from one task to another.
type DAGEdge struct {
	FromTaskID string `json:"from_task_id"`
	ToTaskID   string `json:"to_task_id"`
}

// SurfaceDeclaration names a filesystem surface a task intends to touch.
// AccessMode is "read" or "write".
type SurfaceDeclaration struct {
	Path       string `json:"path"`
	AccessMode string `json:"access_mode"`
}

// VerificationContract defines how a task's output is verified.
type VerificationContract struct {
	Method      string        `json:"method"`
	Commands    []string      `json:"commands"`
	SuccessCode int           `json:"success_code"`
	Timeout     time.Duration `json:"timeout"`
}

// CostAttribution tracks the actual cost incurred by a task.
type CostAttribution struct {
	TokensIn     int      `json:"tokens_in"`
	TokensOut    int      `json:"tokens_out"`
	APICalls     int      `json:"api_calls"`
	EstimatedUSD float64  `json:"estimated_usd"`
	Tier         CostTier `json:"tier"`
}

// Task is the central work unit in the NAVI platform.
type Task struct {
	ID               string               `json:"id"`
	Title            string               `json:"title"`
	Description      string               `json:"description"`
	Status           TaskStatus           `json:"status"`
	Risk             RiskLevel            `json:"risk"`
	AssignedTo       AgentType            `json:"assigned_to"`
	Dependencies     []DAGEdge            `json:"dependencies"`
	Surfaces         []SurfaceDeclaration `json:"surfaces"`
	Verification     VerificationContract `json:"verification"`
	Cost             CostAttribution      `json:"cost"`
	DirectiveID      string               `json:"directive_id,omitempty"`
	ProjectID        string               `json:"project_id,omitempty"`
	WorkspaceID      string               `json:"workspace_id,omitempty"`
	TaskClass        TaskClass            `json:"task_class,omitempty"`
	RawInput         string               `json:"raw_input,omitempty"`
	AcceptanceTarget string               `json:"acceptance_target,omitempty"`
	LifecyclePhase   string               `json:"lifecycle_phase,omitempty"`
	BlockReason      string               `json:"block_reason,omitempty"`
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`
}

// NewTask returns a Task with a generated UUID and pending status.
func NewTask(title, description string, risk RiskLevel, assignedTo AgentType) Task {
	now := time.Now().UTC()
	return Task{
		ID:          uuid.New().String(),
		Title:       title,
		Description: description,
		Status:      TaskStatusPending,
		Risk:        risk,
		AssignedTo:  assignedTo,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// Validate applies deterministic schema rules with no LLM involvement.
func (t *Task) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("task: id is required")
	}
	if t.Title == "" {
		return fmt.Errorf("task: title is required")
	}
	switch t.Risk {
	case RiskLow, RiskMedium, RiskHigh, RiskCritical:
		// valid
	default:
		return fmt.Errorf("task: unknown risk level %q", t.Risk)
	}
	switch t.Status {
	case TaskStatusPending, TaskStatusRunning, TaskStatusBlocked,
		TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled:
		// valid
	default:
		return fmt.Errorf("task: unknown status %q", t.Status)
	}
	if t.TaskClass != "" && !t.TaskClass.IsValid() {
		return fmt.Errorf("task: unknown task class %q", t.TaskClass)
	}
	for _, s := range t.Surfaces {
		if s.Path == "" {
			return fmt.Errorf("task: surface declaration has empty path")
		}
		if s.AccessMode != "read" && s.AccessMode != "write" {
			return fmt.Errorf("task: surface %q has invalid access mode %q", s.Path, s.AccessMode)
		}
	}
	return nil
}

// ScheduledTask defines a future or recurring action the agent has scheduled.
type ScheduledTask struct {
	ID              string    `json:"id"`
	OwnerID         string    `json:"owner_id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	SchedulePattern string    `json:"schedule_pattern"` // e.g. "0 9 * * *" for cron
	Prompt          string    `json:"prompt"`           // What should be in the directive payload
	Status          string    `json:"status"`           // "active", "paused", "completed", "failed"
	LastRunAt       time.Time `json:"last_run_at,omitempty"`
	NextRunAt       time.Time `json:"next_run_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
