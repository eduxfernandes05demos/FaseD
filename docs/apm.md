# Managing standards with APM

This repository uses **[APM (Agent Package Manager)](https://github.com/danielmeppiel/apm)** for managing engineering standards. APM provides:

- ✅ **Zero-config setup** - `apm install` reads `apm.yml` and installs all dependencies
- ✅ **Semantic versioning** - Lock to specific versions or use latest
- ✅ **Automatic AGENTS.md** - `apm compile` generates guardrails from all packages
- ✅ **Mix any standards** - Combine Microsoft, community, and custom packages
- ✅ **One-command updates** - `apm update` to get latest standards

## Built-in standards

By default, spec2cloud includes:
- **azure-standards** - General engineering, documentation, agent-first patterns, CI/CD, security

## Adding more standards

Edit `apm.yml` to add technology-specific standards:

```yaml
dependencies:
  apm:
    - EmeaAppGbb/azure-standards@1.0.0
    - EmeaAppGbb/python-backend@1.0.0  # Add Python backend rules
    - EmeaAppGbb/react-frontend@1.0.0  # Add React frontend rules
```

Then run:

```bash
apm install  # Install new packages
apm compile  # Regenerate AGENTS.md with new standards
```

## Creating custom standards

Create your own APM package:

```bash
my-standards/
├── apm.yml
├── README.md
└── .apm/
    └── instructions/
        ├── api-design.instructions.md
        ├── database-patterns.instructions.md
        └── security-rules.instructions.md
```

Install from any GitHub repo:

```bash
# Public repo
apm install your-org/your-standards

# Private repo (requires GITHUB_APM_PAT)
export GITHUB_APM_PAT=your_token
apm install your-org/private-standards
```

## Updating standards

```bash
# Update all packages to latest
apm update

# Update specific package
apm update danielmeppiel/azure-standards

# Regenerate AGENTS.md after updates
apm compile
```

Back to [docs index](index.md).
