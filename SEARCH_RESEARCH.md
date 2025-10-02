# Documentation Search Research

## Executive Summary

Research into best practices for static site documentation search, with focus on DuckDB's implementation and industry-standard solutions.

## DuckDB's Implementation

**Technology Stack:**
- **Search Library:** [MiniSearch](https://lucaong.github.io/minisearch/) - Tiny client-side full-text search
- **Static Site Generator:** Jekyll
- **Architecture:** Client-side search with lazy-loaded JSON index

**Key Features:**
- Field-based indexing: title, text, category, blurb
- Smart boosting: title (100x), category (20x), blurb (2x)
- Fuzzy matching (0.2 tolerance)
- Prefix search
- Context-aware filtering (docs vs blog)
- ~20 results displayed
- Query term highlighting
- Lazy-loads search index only when needed

**Files:**
- `js/minisearch.js` - Core search library
- `js/search.js` - Custom implementation
- `_includes/searchoverlay.html` - Search UI
- Search index generated as JSON during build

## Alternative Solutions

### 1. Pagefind (Recommended for md2html)

**Pros:**
- Zero-config for most static sites
- Extremely low bandwidth (100-300kB for 10k pages)
- Built-in UI and API
- Rust-based indexer (fast)
- Multilingual support
- No infrastructure needed
- MIT license

**Cons:**
- Requires post-build indexing step
- Newer project (less mature than Algolia)

**Best For:**
- Large sites with bandwidth concerns
- Self-hosted documentation
- Simple integration needs

### 2. Algolia DocSearch

**Pros:**
- Enterprise-grade search
- Free for open-source docs
- Sub-20ms response times
- Hosted infrastructure
- Advanced features (synonyms, analytics)
- Battle-tested

**Cons:**
- Requires approval for free tier
- Cloud dependency
- More complex setup
- Data leaves your infrastructure

**Best For:**
- High-traffic documentation
- Need for analytics
- Want managed solution

### 3. MiniSearch (DuckDB's Choice)

**Pros:**
- Tiny (~9KB minified)
- Pure JavaScript
- Full control over implementation
- No dependencies
- Works offline

**Cons:**
- Requires custom index generation
- Manual UI implementation
- More development effort
- Index size grows with content

**Best For:**
- Custom search experiences
- Small to medium sites
- Developers wanting full control

## Comparison Matrix

| Feature | Pagefind | Algolia DocSearch | MiniSearch |
|---------|----------|-------------------|------------|
| Setup Complexity | Low | Medium | High |
| Bundle Size | ~100KB | ~50KB + API | ~9KB + index |
| Index Generation | Automatic | Crawler | Manual |
| Bandwidth | Very Low | Low | Medium |
| Customization | Medium | Low | High |
| Infrastructure | None | Cloud | None |
| Cost | Free | Free (OSS) | Free |

## Recommendation for md2html

### Primary: Pagefind

**Rationale:**
1. **Fits md2html's philosophy** - Static, self-contained, no infrastructure
2. **Low bandwidth** - Critical for versioned docs (multiple version indexes)
3. **Easy integration** - Post-build indexing step
4. **Go integration** - Can exec Pagefind binary from Go
5. **Version-aware** - Can generate separate indexes per version

**Implementation Plan:**
1. Add `--search` flag to enable search
2. Run Pagefind after HTML generation
3. For versioned docs: index each version separately
4. Include Pagefind UI in templates
5. Optional: Custom UI using Pagefind's JS API

### Alternative: MiniSearch (DuckDB's Approach)

**When to Use:**
- Want zero external dependencies
- Need full control over search behavior
- Building custom search UI anyway
- Index size is manageable

**Implementation Plan:**
1. Generate search index JSON during HTML generation
2. Include MiniSearch library
3. Build custom search UI (similar to DuckDB)
4. Implement field boosting and fuzzy matching
5. For versioned docs: separate indexes or tagged documents

## Technical Integration

### Pagefind Integration

```go
// After generating static HTML
func addSearch(outputDir string) error {
    // Download/use Pagefind binary
    cmd := exec.Command("npx", "-y", "pagefind",
        "--site", outputDir,
        "--output-path", filepath.Join(outputDir, "pagefind"))
    return cmd.Run()
}
```

### Template Changes

```html
<!-- In layout template -->
<head>
    <!-- ... -->
    {{if .EnableSearch}}
    <link href="/pagefind/pagefind-ui.css" rel="stylesheet">
    <script src="/pagefind/pagefind-ui.js"></script>
    {{end}}
</head>
<body>
    {{if .EnableSearch}}
    <div id="search"></div>
    <script>
        window.addEventListener('DOMContentLoaded', () => {
            new PagefindUI({ element: "#search" });
        });
    </script>
    {{end}}
    <!-- ... -->
</body>
```

### Configuration Options

```go
type Config struct {
    // ... existing fields
    EnableSearch      bool
    SearchIndexPath   string // Default: "pagefind"
    SearchEngine      string // "pagefind" or "minisearch"
    SearchExclude     []string // Patterns to exclude from search
}
```

## Next Steps

1. Prototype Pagefind integration
2. Test with versioned documentation
3. Implement search UI in templates
4. Add configuration flags
5. Document search features
6. Consider adding MiniSearch as alternative later

## References

- [Pagefind Documentation](https://pagefind.app/)
- [MiniSearch GitHub](https://github.com/lucaong/minisearch)
- [DuckDB Web Source](https://github.com/duckdb/duckdb-web)
- [Algolia DocSearch](https://docsearch.algolia.com/)
