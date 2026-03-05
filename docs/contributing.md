# Contributing to Spec2Cloud

Thank you for your interest in contributing to Spec2Cloud!

## Development Workflow

1. **Fork the repository**
2. **Create a feature branch**
3. **Make your changes**
4. **Write tests**
5. **Submit a pull request**

## Coding Standards

Please refer to the `AGENTS.md` file in the root directory for detailed coding standards.

### Backend (.NET)

- Follow C# coding conventions
- Use async/await for I/O operations
- Write unit tests with ≥85% coverage
- Use dependency injection

### Frontend (Next.js/React)

- Use TypeScript for type safety
- Follow React best practices
- Use functional components and hooks
- Write component tests

### Documentation

- Write clear, concise documentation
- Use MkDocs format (Markdown)
- Include code examples
- Keep documentation up-to-date

## Testing

### Running Tests

#### Backend Tests

```bash
dotnet test
```

#### Frontend Tests

```bash
cd frontend
npm test
```

### Writing Tests

- Test all public methods and functions
- Test edge cases and error conditions
- Aim for ≥85% code coverage
- Use descriptive test names

## Documentation

### Building Documentation

```bash
pip install -r requirements-docs.txt
mkdocs build --strict
```

### Serving Documentation Locally

```bash
mkdocs serve
```

Visit `http://localhost:8000` to view the documentation.

## Pull Request Process

1. **Update documentation** - Ensure all changes are documented
2. **Run tests** - All tests must pass
3. **Code review** - Address review comments
4. **Merge** - Maintainers will merge approved PRs

## Quality Checklist

Before submitting a pull request:

- ✅ All tests pass
- ✅ Code coverage ≥85%
- ✅ Type safety enforced
- ✅ Linters and formatters pass
- ✅ Documentation updated
- ✅ No secrets committed
- ✅ Code follows team standards

## Questions?

If you have questions, please:

1. Check existing documentation
2. Search existing issues
3. Open a new issue with the `question` label
