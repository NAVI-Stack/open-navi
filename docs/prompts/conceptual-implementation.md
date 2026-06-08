## Objective

Perform a **complete gap analysis** between the conceptual system design and the current codebase implementation, then update the implementation plan to reflect only the **remaining work required**.

---

# Inputs

Conceptual design specification 

```
@docs/canonical/conceptual-design-overview.md
```

Existing plan document

```
@.cursor/plans/concept-vs-implementation-gap-analysis.plan.md
```

Repository source code.

---

# Phase 1 — Build the Conceptual Implementation Checklist

Parse `conceptual-design-overview.md` and construct a **canonical checklist of all implementation requirements**.

Extract and enumerate:

* Core system components
* Services or modules
* Domain entities
* Workflows / pipelines
* APIs or interfaces
* Integration points
* Infrastructure concerns
* Cross-cutting concerns (logging, auth, config, etc.)

Create an internal structured checklist:

```
Concept ID
Concept Name
Description
Expected Implementation Location (if implied)
Dependencies
```

Every concept must receive a **unique identifier**.

This checklist becomes the **source of truth for the audit**.

---

# Phase 2 — Audit the Codebase

For each concept in the checklist:

Search the repository to determine whether the concept is implemented.

Classify status as one of:

```
complete
partial
missing
```

Definitions:

**complete**

* Fully implemented
* Integrated with related components
* No remaining work implied by the conceptual design

**partial**

* Some code exists but functionality is incomplete
* Missing integrations, behaviors, or edge cases

**missing**

* No implementation exists

Record supporting evidence:

```
files
modules
classes
functions
interfaces
```

---

# Phase 3 — Identify Implementation Gaps

For every concept marked **partial** or **missing**, determine:

* What is not implemented
* What functionality is incomplete
* What integration points are missing
* What architectural constraints are unmet

---

# Phase 4 — Update the Plan File

Modify only:

```
@.cursor/plans/concept-vs-implementation-gap-analysis.plan.md
```

Apply the following rules:

### Remove

Delete all tasks corresponding to concepts that are **100% complete**.

### Retain

Keep tasks for concepts that are **partially implemented**.

Update them to reflect the **remaining work only**.

### Add

Create tasks for **missing concepts**.

---

# Task Format (Required)

Each task must follow this structure:

```
## [Concept ID] Concept Name

Status: partial | missing

Concept Summary
Short description from conceptual design.

Current Implementation
Files/modules that partially implement this concept (if any).

Gap
What functionality is not yet implemented.

Required Work
Concrete implementation steps required.

Relevant Files
- path/to/file
- path/to/module
```

---

# Phase 5 — Self-Verification

Before finishing:

1. Confirm **every concept from the checklist was evaluated**.
2. Confirm **no completed concepts remain in the plan**.
3. Confirm **all missing concepts have tasks**.
4. Confirm the plan represents **100% of remaining work required to reach conceptual parity**.

If any conceptual element lacks evaluation, repeat the audit.

---

# Output Constraint

Only modify:

```
@.cursor/plans/concept-vs-implementation-gap-analysis.plan.md
```

Do not modify any other files.

---

# Expected Result

The updated plan file becomes a **complete implementation backlog** representing the **exact remaining work required to align the codebase with the conceptual design**.