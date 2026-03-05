# Example usage

## Greenfield example (new project)

```bash
# Start with your product idea
"I want to create a smart AI agent for elderly care that tracks vitals and alerts caregivers"

# Step 1: Create the PRD
/prd

# Step 2: Break down into features
/frd

# Step 3: Generate agent guidelines from standards (optional, can defer)
/generate-agents

# Step 4: Create technical plans
/plan

# Step 5a: Implement locally
/implement

# OR Step 5b: Delegate to GitHub Copilot
/delegate

# Step 6: Deploy to Azure
/deploy
```

## Brownfield example (existing project)

```bash
# You have an existing codebase with minimal or outdated documentation
"I inherited a marketing campaign management app built in Python/React"

# Step 1: Reverse engineer technical tasks from code
/plan-brown
# Agent analyzes codebase (Python FastAPI backend, React frontend)
# Creates specs/tasks/ with honest documentation of what exists
# Notes: "Task 008: Email service - stub only, not fully implemented"

# Step 2: Synthesize feature requirements from tasks
/frd-brown
# Agent groups tasks into product features
# Creates specs/features/campaign-management.md, user-authentication.md, etc.
# Notes: "Email notifications feature - partially implemented"

# Step 3: Create product vision from features
/prd-brown
# Agent synthesizes overall product purpose
# Creates specs/prd.md with goals, scope, user stories
# Includes "Product Status Assessment" with gaps and recommendations

# Result: Complete documentation traceability
# PRD → FRDs → Tasks → Code (with file paths)

# Optional: Now enhance using greenfield workflow
/frd  # Add new features to existing FRDs
/plan # Create tasks for new features
/implement # Build the enhancements
```

Back to [docs index](index.md).
