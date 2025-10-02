# Versioned Documentation Support

md2html now supports versioned documentation using git tags and branches.

## Usage

Enable versioned docs mode with the `-versions` flag:

```bash
# Serve docs with version support (uses git tags)
md2html -http :8080 -versions

# Include branches as versions too
md2html -http :8080 -versions -version-branches

# Filter tags by pattern
md2html -http :8080 -versions -version-pattern 'v*'

# Set a default version
md2html -http :8080 -versions -version-default v1.0.0
```

## How It Works

When versioning is enabled, md2html:

1. **Discovers versions** - Scans your git repository for tags (and optionally branches)
2. **Serves versioned content** - Fetches file content from specific git refs using `git show`
3. **Provides version switcher** - Adds a UI dropdown to switch between versions
4. **URL-based routing** - Uses `/v/{version}/{path}` URL format

## URL Structure

- `/` - Default version (or current if no default specified)
- `/v/{version}/{path}` - Specific version of a document
- `/api/versions` - JSON API endpoint listing all versions

## Examples

```bash
# View current version
http://localhost:8080/

# View v1.0.0 version of README
http://localhost:8080/v/v1.0.0/README

# View v2.0.0 version of docs/guide
http://localhost:8080/v/v2.0.0/docs/guide

# List all available versions (JSON)
curl http://localhost:8080/api/versions
```

## Features

- **Automatic version detection** - Scans git tags and branches
- **Version filtering** - Use patterns to filter which tags to show
- **Tag/branch distinction** - UI shows whether version is a tag or branch
- **Live switching** - JavaScript-based version switcher with clean URLs
- **API access** - RESTful API for version information

## API Response

The `/api/versions` endpoint returns:

```json
{
  "versions": [
    {
      "Name": "v2.0.0",
      "Ref": "refs/tags/v2.0.0",
      "IsTag": true,
      "Commit": "abc1234"
    },
    {
      "Name": "main",
      "Ref": "origin/main",
      "IsTag": false,
      "Commit": "def5678"
    }
  ],
  "current_version": "main",
  "default_version": "v2.0.0"
}
```

## Configuration Flags

- `-versions` - Enable versioned documentation mode
- `-version-pattern` - Git tag pattern to match (default: `*`)
- `-version-branches` - Include branches as versions (default: false)
- `-version-default` - Default version to display (default: latest tag or current)

## Requirements

- Must be run inside a git repository
- Git must be available in PATH
- Repository should have tags (or branches with `-version-branches`)

## Use Cases

### Software Project Documentation

```bash
# Show docs for all released versions
md2html -http :8080 -versions -version-pattern 'v*'
```

### API Documentation

```bash
# Include development branches
md2html -http :8080 -versions -version-branches
```

### Release Documentation

```bash
# Show only release tags, default to latest
md2html -http :8080 -versions -version-pattern 'release-*' -version-default release-2.0
```
