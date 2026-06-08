package schema

import "time"

// SandboxRuntime identifies the execution backend used for a sandbox profile.
type SandboxRuntime string

const (
	SandboxRuntimeDocker SandboxRuntime = "docker"
)

var validSandboxRuntimes = map[SandboxRuntime]struct{}{
	SandboxRuntimeDocker: {},
}

func (r SandboxRuntime) IsValid() bool {
	_, ok := validSandboxRuntimes[r]
	return ok
}

func ParseSandboxRuntime(value string) (SandboxRuntime, error) {
	return parseWorldModelEnum(value, validSandboxRuntimes, "sandbox_runtime")
}

// SandboxProfileStatus is the lifecycle state for an execution sandbox profile.
type SandboxProfileStatus string

const (
	SandboxProfileStatusActive   SandboxProfileStatus = "active"
	SandboxProfileStatusInactive SandboxProfileStatus = "inactive"
	SandboxProfileStatusArchived SandboxProfileStatus = "archived"
)

var validSandboxProfileStatuses = map[SandboxProfileStatus]struct{}{
	SandboxProfileStatusActive:   {},
	SandboxProfileStatusInactive: {},
	SandboxProfileStatusArchived: {},
}

func (s SandboxProfileStatus) IsValid() bool {
	_, ok := validSandboxProfileStatuses[s]
	return ok
}

func ParseSandboxProfileStatus(value string) (SandboxProfileStatus, error) {
	return parseWorldModelEnum(value, validSandboxProfileStatuses, "sandbox_profile_status")
}

// SandboxNetworkMode controls container network egress.
type SandboxNetworkMode string

const (
	SandboxNetworkNone   SandboxNetworkMode = "none"
	SandboxNetworkBridge SandboxNetworkMode = "bridge"
)

var validSandboxNetworkModes = map[SandboxNetworkMode]struct{}{
	SandboxNetworkNone:   {},
	SandboxNetworkBridge: {},
}

func (m SandboxNetworkMode) IsValid() bool {
	_, ok := validSandboxNetworkModes[m]
	return ok
}

func ParseSandboxNetworkMode(value string) (SandboxNetworkMode, error) {
	return parseWorldModelEnum(value, validSandboxNetworkModes, "sandbox_network_mode")
}

// SandboxProfile defines a bounded execution environment for project coding
// validation. V1 keeps it intentionally small: one workspace mount target,
// a command allowlist, an env allowlist, and conservative runtime limits.
type SandboxProfile struct {
	ID                   string               `json:"sandbox_profile_id"`
	Name                 string               `json:"name"`
	Description          string               `json:"description,omitempty"`
	Runtime              SandboxRuntime       `json:"runtime"`
	Status               SandboxProfileStatus `json:"status"`
	Image                string               `json:"image"`
	NetworkMode          SandboxNetworkMode   `json:"network_mode"`
	WorkspaceMountTarget string               `json:"workspace_mount_target"`
	CommandAllowlist     []string             `json:"command_allowlist,omitempty"`
	EnvAllowlist         []string             `json:"env_allowlist,omitempty"`
	DefaultTimeoutMS     int                  `json:"default_timeout_ms"`
	MaxTimeoutMS         int                  `json:"max_timeout_ms"`
	CPULimit             string               `json:"cpu_limit,omitempty"`
	MemoryLimit          string               `json:"memory_limit,omitempty"`
	CreatedAt            time.Time            `json:"created_at"`
	UpdatedAt            time.Time            `json:"updated_at"`
	CreatedBy            string               `json:"created_by"`
	Metadata             string               `json:"metadata,omitempty"`
}
