# Spec2Cloud Documentation

This directory contains the MkDocs-based documentation for Spec2Cloud.

## Running the Documentation Server

### Prerequisites

Install Python 3.8 or higher.

### Installation

1. Install documentation dependencies:

```bash
pip install -r requirements-docs.txt
```

### Serve Locally

To run the documentation server locally:

```bash
mkdocs serve
```

The documentation will be available at `http://localhost:8000`

The server supports live-reload, so any changes you make to the documentation files will automatically refresh in your browser.

### Build Static Site

To build the static HTML documentation:

```bash
mkdocs build
```

The built site will be in the `site/` directory.

### Build with Strict Mode

To ensure all links and references are valid:

```bash
mkdocs build --strict
```

This will fail if there are any warnings, helping ensure documentation quality.

## Documentation Structure

- `index.md` - Home page
- `getting-started.md` - Getting started guide
- `benefits.md` - Benefits of using Spec2Cloud
- `architecture.md` - System architecture
- `contributing.md` - Contribution guidelines

## Writing Documentation

- Use Markdown format
- Follow the MkDocs style guide
- Include code examples where appropriate
- Keep content up-to-date
- Test with `mkdocs build --strict` before committing

## Configuration

The MkDocs configuration is in `mkdocs.yml` at the root of the repository.
