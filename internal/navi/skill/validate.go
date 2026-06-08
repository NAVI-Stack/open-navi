package skill

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/open-navi/navi/internal/capability"
)

const (
	defaultPythonVersion   = "3.11"
	defaultVenvStrategy    = "per_category"
	defaultPythonLifecycle = "cold"
	defaultPythonIOFormat  = "json_stdio"
	defaultSpawnTimeoutMS  = 5000
	defaultMaxInputBytes   = 1024 * 1024
	defaultMaxOutputBytes  = 5 * 1024 * 1024
	defaultPythonTimeoutMS = 30000
	defaultMaxMemoryMB     = 256
	defaultTmpDirMB        = 64
	defaultWarmPoolSize    = 1
	defaultPythonMaturity  = "prototype"
	defaultSubprocessProto = "jsonrpc_stdio"
)

func applySpecDefaults(spec *OSS27Spec) {
	if spec == nil {
		return
	}
	if spec.PythonRuntime != nil {
		rt := spec.PythonRuntime
		if rt.PythonVersion == "" {
			rt.PythonVersion = defaultPythonVersion
		}
		if rt.VenvStrategy == "" {
			rt.VenvStrategy = defaultVenvStrategy
		}
		if rt.RequirementsFile == "" {
			rt.RequirementsFile = "requirements.txt"
		}
		if rt.Lifecycle == "" {
			rt.Lifecycle = defaultPythonLifecycle
		}
		if rt.Lifecycle == "warm" && rt.WarmPoolSize <= 0 {
			rt.WarmPoolSize = defaultWarmPoolSize
		}
		if rt.SpawnTimeoutMS <= 0 {
			rt.SpawnTimeoutMS = defaultSpawnTimeoutMS
		}
		if rt.IOFormat == "" {
			rt.IOFormat = defaultPythonIOFormat
		}
		if rt.MaxInputBytes <= 0 {
			rt.MaxInputBytes = defaultMaxInputBytes
		}
		if rt.MaxOutputBytes <= 0 {
			rt.MaxOutputBytes = defaultMaxOutputBytes
		}
		if rt.TimeoutMS <= 0 {
			if spec.Performance.TimeoutMS > 0 {
				rt.TimeoutMS = spec.Performance.TimeoutMS
			} else {
				rt.TimeoutMS = defaultPythonTimeoutMS
			}
		}
		if rt.MaxMemoryMB <= 0 {
			rt.MaxMemoryMB = defaultMaxMemoryMB
		}
		if rt.TmpdirMB == 0 {
			rt.TmpdirMB = defaultTmpDirMB
		}
		if rt.Maturity == "" {
			rt.Maturity = defaultPythonMaturity
		}
	}
	for i := range spec.Interfaces {
		if spec.Interfaces[i].Transport.Type == "subprocess" && spec.Interfaces[i].Transport.Protocol == "" {
			spec.Interfaces[i].Transport.Protocol = defaultSubprocessProto
		}
	}
}

func validateSpec(spec *OSS27Spec) error {
	if spec == nil {
		return nil
	}
	if spec.SkillID == "" {
		return fmt.Errorf("missing skill_id")
	}
	if len(spec.Interfaces) == 0 {
		return fmt.Errorf("missing interfaces")
	}
	if spec.Display.Name == "" {
		return fmt.Errorf("missing display.name")
	}
	if spec.Display.Description == "" {
		return fmt.Errorf("missing display.description")
	}
	if spec.Effects.RiskTier != "" && !isOneOf(spec.Effects.RiskTier, "low", "medium", "high") {
		return fmt.Errorf("invalid effects.risk_tier %q", spec.Effects.RiskTier)
	}
	if spec.Effects.IdempotencyLevel != "" && !isOneOf(spec.Effects.IdempotencyLevel, "idempotent", "non_idempotent", "conditional") {
		return fmt.Errorf("invalid effects.idempotency_level %q", spec.Effects.IdempotencyLevel)
	}
	if spec.Effects.Reversibility != "" && !isOneOf(spec.Effects.Reversibility, "reversible_internal", "compensable_external", "irreversible") {
		return fmt.Errorf("invalid effects.reversibility %q", spec.Effects.Reversibility)
	}
	if spec.Governance.TrustTier != "" && !isOneOf(spec.Governance.TrustTier, "builtin", "verified", "community", "local") {
		return fmt.Errorf("invalid governance.trust_tier %q", spec.Governance.TrustTier)
	}
	if spec.Reliability.RetryPolicy != "" && !isOneOf(spec.Reliability.RetryPolicy, "none", "safe", "conditional") {
		return fmt.Errorf("invalid reliability.retry_policy %q", spec.Reliability.RetryPolicy)
	}
	if spec.Effects.Reversibility == "irreversible" && !spec.Effects.RequiresConfirmation {
		return fmt.Errorf("irreversible skills must require confirmation")
	}

	for i := range spec.Interfaces {
		iface := &spec.Interfaces[i]
		if iface.Name == "" {
			return fmt.Errorf("interface[%d] missing name", i)
		}
		switch iface.Transport.Type {
		case "internal":
		case "rest":
			if iface.Transport.REST == nil || strings.TrimSpace(iface.Transport.REST.URL) == "" {
				return fmt.Errorf("interface %q uses rest transport without rest.url", iface.Name)
			}
			if len(spec.Security.Sandbox.NetworkEgress) == 0 {
				return fmt.Errorf("rest transport requires security.sandbox.network_egress")
			}
		case "mcp_tool":
			if iface.Transport.MCP == nil || iface.Transport.MCP.Server == "" || iface.Transport.MCP.Tool == "" {
				return fmt.Errorf("interface %q uses mcp_tool transport without mcp.server/tool", iface.Name)
			}
			if len(spec.Security.Sandbox.NetworkEgress) == 0 {
				return fmt.Errorf("mcp_tool transport requires security.sandbox.network_egress")
			}
		case "subprocess_python":
			if spec.PythonRuntime == nil {
				return fmt.Errorf("interface %q uses subprocess_python without python_runtime", iface.Name)
			}
			if iface.Transport.SubprocessPython == nil || strings.TrimSpace(iface.Transport.SubprocessPython.Entrypoint) == "" {
				return fmt.Errorf("interface %q uses subprocess_python without subprocess_python.entrypoint", iface.Name)
			}
			if spec.PythonRuntime.NetworkAccess && len(spec.Security.Sandbox.NetworkEgress) == 0 {
				return fmt.Errorf("subprocess_python with network_access requires security.sandbox.network_egress")
			}
		case "subprocess":
			if strings.TrimSpace(iface.Transport.Runtime) == "" {
				return fmt.Errorf("interface %q uses subprocess without transport.runtime", iface.Name)
			}
			if len(iface.Transport.Command) == 0 || strings.TrimSpace(iface.Transport.Command[0]) == "" {
				return fmt.Errorf("interface %q uses subprocess without transport.command", iface.Name)
			}
			if protocol := strings.TrimSpace(iface.Transport.Protocol); protocol != "" && protocol != defaultSubprocessProto {
				return fmt.Errorf("interface %q uses unsupported subprocess protocol %q", iface.Name, iface.Transport.Protocol)
			}
		default:
			return fmt.Errorf("interface %q has unsupported transport %q", iface.Name, iface.Transport.Type)
		}
	}
	for i, surface := range spec.UISurfaces {
		result := capability.ValidateUISurface(surface, "")
		if !result.Valid {
			return fmt.Errorf("ui_surfaces[%d]: %s", i, strings.Join(result.Reasons, "; "))
		}
	}

	if spec.Effects.RiskTier == "high" && !hasSecurityMetadata(spec) {
		return fmt.Errorf("high-risk skills require security metadata")
	}

	return nil
}

// ApplySpecDefaults exposes the defaulting logic for OSS27Spec so it can be
// reused by callers outside this package (e.g. HTTP APIs and CLI helpers).
func ApplySpecDefaults(spec *OSS27Spec) {
	applySpecDefaults(spec)
}

// ValidateSpec exposes the validator for OSS27Spec so it can be reused by
// HTTP APIs and CLI helpers. It enforces the canonical rules defined in
// docs/canonical/skills.md.
func ValidateSpec(spec *OSS27Spec) error {
	return validateSpec(spec)
}

func hasSecurityMetadata(spec *OSS27Spec) bool {
	if spec == nil {
		return false
	}
	if len(spec.Security.Auth) > 0 {
		return true
	}
	if spec.Security.DataAccess.PII != "" || spec.Security.DataAccess.Secrets != "" {
		return true
	}
	if spec.Security.Sandbox.Required || len(spec.Security.Sandbox.NetworkEgress) > 0 {
		return true
	}
	return false
}

func ensurePythonVersion(required string) error {
	if required == "" {
		return nil
	}
	pythonExe, err := resolvePythonExecutable("")
	if err != nil {
		return err
	}
	out, err := exec.Command(pythonExe, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("check python version: %w", err)
	}
	actual := extractPythonVersion(string(out))
	if actual == "" {
		return fmt.Errorf("unable to parse python version from %q", strings.TrimSpace(string(out)))
	}
	if compareVersionStrings(actual, required) < 0 {
		return fmt.Errorf("python %s is below required version %s", actual, required)
	}
	return nil
}

func isOneOf(v string, allowed ...string) bool {
	for _, item := range allowed {
		if v == item {
			return true
		}
	}
	return false
}

func extractPythonVersion(raw string) string {
	for _, field := range strings.Fields(raw) {
		if startsWithDigit(field) {
			field = strings.TrimSpace(field)
			field = strings.Trim(field, "(),")
			return field
		}
	}
	return ""
}

func compareVersionStrings(actual, required string) int {
	a := versionParts(actual)
	b := versionParts(required)
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		var av, bv int
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		switch {
		case av < bv:
			return -1
		case av > bv:
			return 1
		}
	}
	return 0
}

func versionParts(v string) []int {
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		digits := make([]rune, 0, len(part))
		for _, r := range part {
			if r >= '0' && r <= '9' {
				digits = append(digits, r)
				continue
			}
			break
		}
		if len(digits) == 0 {
			break
		}
		n, err := strconv.Atoi(string(digits))
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out
}

func startsWithDigit(s string) bool {
	if s == "" {
		return false
	}
	b := s[0]
	return b >= '0' && b <= '9'
}

func resolvePythonExecutable(preferred string) (string, error) {
	candidates := []string{}
	if preferred != "" {
		candidates = append(candidates, preferred)
	}
	candidates = append(candidates, "python", "python3", "py")
	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate)
		if err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("python executable not found on PATH")
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
