# Shell baselines (Greenfield)

Spec2Cloud supports a “shell-first” greenfield approach: start from a predefined baseline (“shell”), then use coding agents to translate natural language requirements into the missing code, wiring, and configuration.

## Available shells

- .NET shell: https://github.com/EmeaAppGbb/shell-dotnet
- Agentic .NET shell: https://github.com/EmeaAppGbb/agentic-shell-dotnet
- Agentic Python shell: https://github.com/EmeaAppGbb/agentic-shell-python

## How to use shells with agents

1. Pick a shell repo that matches your language/runtime.
2. Add your requirements in plain language (and/or run the Spec2Cloud prompts).
3. Let agents iteratively implement the gaps: endpoints, UI, storage, tests, and deployment.

Back to [docs index](index.md).
