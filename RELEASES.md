# Release Management

This project supports multiple approaches for creating releases with semantic versioning.

## Option 1: Auto-Release with Conventional Commits (Recommended)

The project includes an auto-release workflow that automatically creates releases based on your commit messages using [Conventional Commits](https://www.conventionalcommits.org/).

### How it works:
- Every push to `main` triggers the auto-release workflow
- The workflow analyzes commit messages since the last release
- It automatically increments the version based on commit types:
  - `fix:` → patch version (v1.0.0 → v1.0.1)
  - `feat:` → minor version (v1.0.0 → v1.1.0)
  - `BREAKING CHANGE:` or `!` → major version (v1.0.0 → v2.0.0)

### Commit Message Examples:
```bash
# Patch release (bug fixes)
git commit -m "fix: resolve database connection timeout"

# Minor release (new features)  
git commit -m "feat: add support for HTTP/3 monitoring"

# Major release (breaking changes)
git commit -m "feat!: change API response format"
# or
git commit -m "feat: change API response format

BREAKING CHANGE: The response format has changed from XML to JSON"
```

### Skip auto-release:
To skip auto-release for a commit, include `skip-release` in the commit message:
```bash
git commit -m "docs: update README [skip-release]"
```

## Option 2: Manual Release Script

Use the provided script for manual control:

```bash
# Patch release (v1.0.0 → v1.0.1)
./scripts/release.sh patch

# Minor release (v1.0.0 → v1.1.0)  
./scripts/release.sh minor

# Major release (v1.0.0 → v2.0.0)
./scripts/release.sh major

# Specific version
./scripts/release.sh v1.2.3
```

The script will:
1. Check that your working directory is clean
2. Show current version and proposed new version
3. Ask for confirmation
4. Create and push the tag
5. Trigger the build workflow

## Option 3: Manual Git Tags

You can always create releases manually:

```bash
# Create a tag
git tag v1.0.0

# Push the tag
git push origin v1.0.0
```

## What Happens After Creating a Release

Regardless of which method you use, when a new version tag is created:

1. **Build Workflow Triggers**: Automatically builds binaries for all supported platforms:
   - Linux x86_64, ARM64, ARMv7, RISC-V 64
   - Windows x86_64
   - macOS ARM64

2. **GitHub Release Created**: A GitHub release is automatically created with:
   - All platform binaries
   - SHA256 checksums
   - Release notes
   - Installation instructions

3. **Artifacts Available**: Binaries are immediately available for download from the GitHub releases page.

## Semantic Versioning

All versions follow [Semantic Versioning](https://semver.org/):
- **MAJOR**: Incompatible API changes
- **MINOR**: Backward-compatible functionality additions  
- **PATCH**: Backward-compatible bug fixes

## First Release

If you haven't created any releases yet, the first tag will be `v0.1.0` (for conventional commits) or you can manually create `v1.0.0`.

## Choosing an Approach

**For a single developer workflow:**
- **Auto-release** is recommended if you want to focus on coding and let the system handle releases
- **Manual script** is good if you want control over when releases happen
- **Manual tags** if you prefer maximum control and simplicity

All approaches can be used together - the auto-release can be disabled by including `[skip-release]` in commit messages when you want manual control. 