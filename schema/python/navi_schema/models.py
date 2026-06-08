"""NAVI AI Platform — Pydantic V2 Models

Generated from JSON Schema definitions in schema/jsonschema/.
To regenerate: cd schema/python && python generate.py

This file can be hand-edited for initial development and then overwritten
by the generation pipeline once datamodel-code-generator is installed.
"""

from __future__ import annotations

from datetime import datetime
from enum import Enum
from typing import Any, Optional

from pydantic import BaseModel, Field


# ---------------------------------------------------------------------------
# Enums (mirrors internal/schema/*.go const blocks)
# ---------------------------------------------------------------------------


class RiskLevel(str, Enum):
    """Blast radius classification for a task."""
    LOW = "low"
    MEDIUM = "medium"
    HIGH = "high"
    CRITICAL = "critical"


class TaskStatus(str, Enum):
    """Lifecycle state machine for a task."""
    PENDING = "pending"
    RUNNING = "running"
    BLOCKED = "blocked"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


class EventKind(str, Enum):
    """Commands express intent; facts record what happened."""
    COMMAND = "command"
    FACT = "fact"


class EventType(str, Enum):
    """Fully-qualified event name on the bus."""
    # Task commands
    CMD_TASK_CREATE = "cmd.task.create"
    CMD_TASK_ASSIGN = "cmd.task.assign"
    CMD_TASK_CANCEL = "cmd.task.cancel"
    CMD_TASK_UPDATE = "cmd.task.update"
    # Claim commands
    CMD_CLAIM_ACQUIRE = "cmd.claim.acquire"
    CMD_CLAIM_RELEASE = "cmd.claim.release"
    # Agent commands
    CMD_AGENT_REGISTER = "cmd.agent.register"
    CMD_AGENT_RETIRE = "cmd.agent.retire"
    # Task facts
    FACT_TASK_CREATED = "fact.task.created"
    FACT_TASK_ASSIGNED = "fact.task.assigned"
    FACT_TASK_STARTED = "fact.task.started"
    FACT_TASK_COMPLETED = "fact.task.completed"
    FACT_TASK_FAILED = "fact.task.failed"
    FACT_TASK_CANCELLED = "fact.task.cancelled"
    # Claim facts
    FACT_CLAIM_ACQUIRED = "fact.claim.acquired"
    FACT_CLAIM_RELEASED = "fact.claim.released"
    FACT_CLAIM_EXPIRED = "fact.claim.expired"
    FACT_CLAIM_CONFLICT = "fact.claim.conflict"
    # Agent facts
    FACT_AGENT_REGISTERED = "fact.agent.registered"
    FACT_AGENT_RETIRED = "fact.agent.retired"
    # Cost facts
    FACT_COST_RECORDED = "fact.cost.recorded"
    # Governor facts
    FACT_GOVERNOR_TRIPPED = "fact.governor.tripped"


class AgentType(str, Enum):
    """Role of an agent in the system."""
    ORCHESTRATOR = "orchestrator"
    ARCHITECT = "architect"
    DEVELOPER = "coder"
    REVIEWER = "reviewer"
    TESTER = "tester"
    DEVOPS = "devops"
    SECOPS = "secops"
    DOCS = "docs"


class ClaimStatus(str, Enum):
    """Lifecycle of a surface claim."""
    ACTIVE = "active"
    RELEASED = "released"
    EXPIRED = "expired"
    CONFLICT = "conflict"


class CostTier(str, Enum):
    """API spending bucket for tier-level caps."""
    FREE = "free"
    CHEAP = "cheap"
    MODERATE = "moderate"
    EXPENSIVE = "expensive"


class Tool(str, Enum):
    """Capability that an agent can invoke."""
    READ_FILE = "read_file"
    WRITE_FILE = "write_file"
    RUN_COMMAND = "run_command"
    WEB_SEARCH = "web_search"
    CALL_LLM = "call_llm"
    GIT_OPS = "git_ops"
    DEPLOY_OPS = "deploy_ops"
    SEC_SCAN = "sec_scan"


# ---------------------------------------------------------------------------
# Task domain (mirrors internal/schema/task.go)
# ---------------------------------------------------------------------------


class DAGEdge(BaseModel):
    """Dependency edge from one task to another."""
    from_task_id: str
    to_task_id: str


class SurfaceDeclaration(BaseModel):
    """Filesystem surface a task intends to touch."""
    path: str
    access_mode: str = Field(pattern=r"^(read|write)$")


class VerificationContract(BaseModel):
    """How a task's output is verified."""
    method: str
    commands: list[str] = Field(default_factory=list)
    success_code: int = 0
    timeout: int = Field(default=0, description="Timeout in nanoseconds (Go time.Duration).")


class CostAttribution(BaseModel):
    """Actual cost incurred by a task."""
    tokens_in: int = 0
    tokens_out: int = 0
    api_calls: int = 0
    estimated_usd: float = 0.0
    tier: CostTier = CostTier.FREE


class Task(BaseModel):
    """Central work unit in the NAVI AI platform."""
    id: str
    title: str = Field(min_length=1)
    description: str = ""
    status: TaskStatus = TaskStatus.PENDING
    risk: RiskLevel = RiskLevel.LOW
    assigned_to: AgentType = AgentType.DEVELOPER
    dependencies: list[DAGEdge] = Field(default_factory=list)
    surfaces: list[SurfaceDeclaration] = Field(default_factory=list)
    verification: Optional[VerificationContract] = None
    cost: Optional[CostAttribution] = None
    created_at: datetime
    updated_at: datetime


# ---------------------------------------------------------------------------
# Claim domain (mirrors internal/schema/claim.go)
# ---------------------------------------------------------------------------


class SurfaceClaim(BaseModel):
    """Agent's exclusive write lock on a directory surface."""
    id: str
    path: str = Field(min_length=1)
    agent_id: str = Field(min_length=1)
    agent_type: AgentType
    status: ClaimStatus = ClaimStatus.ACTIVE
    ttl: int = Field(description="TTL in nanoseconds.")
    acquired_at: datetime
    expires_at: datetime


class ClaimRequest(BaseModel):
    """Input to a claim acquisition."""
    path: str = Field(min_length=1)
    agent_id: str = Field(min_length=1)
    agent_type: AgentType
    ttl: int = Field(description="TTL in nanoseconds.")


class ClaimResponse(BaseModel):
    """Result of a claim acquisition attempt."""
    claim: Optional[SurfaceClaim] = None
    conflict: Optional[SurfaceClaim] = None
    error: Optional[str] = None


# ---------------------------------------------------------------------------
# Agent domain (mirrors internal/schema/agent.go)
# ---------------------------------------------------------------------------


class AgentCapability(BaseModel):
    """What an agent is permitted to do."""
    permitted_tools: list[Tool] = Field(default_factory=list)
    writeable_roots: list[str] = Field(default_factory=list)
    max_autonomous_risk: RiskLevel = RiskLevel.LOW
    cost_tier: CostTier = CostTier.FREE
    max_retries: int = 0
    max_action_budget: int = 0


# ---------------------------------------------------------------------------
# Event domain (mirrors internal/schema/event.go)
# ---------------------------------------------------------------------------

SCHEMA_VERSION = "1.0.0"


class Event(BaseModel):
    """Envelope for every message on the bus."""
    id: str
    type: EventType
    kind: EventKind
    correlation_id: str = Field(min_length=1)
    causal_parent: Optional[str] = None
    source_agent: AgentType
    target_agent: Optional[AgentType] = None
    timestamp: datetime
    payload: Any = None
    schema_version: str = SCHEMA_VERSION


# --- Command payloads ---

class TaskCreatePayload(BaseModel):
    task: Task


class TaskAssignPayload(BaseModel):
    task_id: str
    assign_to: AgentType


class TaskCancelPayload(BaseModel):
    task_id: str
    reason: str


class TaskUpdatePayload(BaseModel):
    task_id: str
    status: TaskStatus


class ClaimAcquirePayload(BaseModel):
    path: str
    agent_id: str
    agent_type: AgentType
    ttl: int


class ClaimReleasePayload(BaseModel):
    claim_id: str


class AgentRegisterPayload(BaseModel):
    agent_id: str
    agent_type: AgentType


class AgentRetirePayload(BaseModel):
    agent_id: str


# --- Fact payloads ---

class TaskCreatedPayload(BaseModel):
    task: Task


class TaskAssignedPayload(BaseModel):
    task_id: str
    assign_to: AgentType


class TaskStartedPayload(BaseModel):
    task_id: str


class TaskCompletedPayload(BaseModel):
    task_id: str
    cost: CostAttribution


class TaskFailedPayload(BaseModel):
    task_id: str
    error: str


class TaskCancelledPayload(BaseModel):
    task_id: str
    reason: str


class ClaimAcquiredPayload(BaseModel):
    claim: SurfaceClaim


class ClaimReleasedPayload(BaseModel):
    claim_id: str


class ClaimExpiredPayload(BaseModel):
    claim_id: str
    path: str


class ClaimConflictPayload(BaseModel):
    requested_path: str
    blocking_claim_id: str


class AgentRegisteredPayload(BaseModel):
    agent_id: str
    agent_type: AgentType


class AgentRetiredPayload(BaseModel):
    agent_id: str


class CostRecordedPayload(BaseModel):
    cost: CostAttribution


class GovernorTrippedPayload(BaseModel):
    governor_type: str
    limit: str
    actual: str
