// Search functionality using MiniSearch
let searchIndex = null;
let searchData = null;
let selectedResultIndex = -1;

// Initialize search when modal is opened
function initSearch() {
    if (searchIndex) return; // Already initialized

    if (!window.MD2HTML_SEARCH_INDEX) {
        console.error('Search index not loaded (window.MD2HTML_SEARCH_INDEX is missing)');
        return;
    }
    searchData = window.MD2HTML_SEARCH_INDEX;

    searchIndex = new MiniSearch({
        fields: ['title', 'text', 'category', 'blurb'],
        storeFields: ['title', 'url', 'category', 'blurb', 'type'],
        searchOptions: {
            boost: { title: 100, category: 20, blurb: 2 },
            fuzzy: 0.2,
            prefix: true
        }
    });
    searchIndex.addAll(searchData.map((doc, id) => ({ id, ...doc })));

    console.log(`Search index loaded: ${searchData.length} documents`);
}

// Open search modal
function openSearch() {
    const overlay = document.getElementById('search-overlay');
    const input = document.getElementById('search-input');

    overlay.style.display = 'flex';
    input.value = '';
    input.focus();
    selectedResultIndex = -1;

    // Initialize search index if not already loaded
    initSearch();

    // Show initial hint
    showSearchHint();
}

// Close search modal
function closeSearch() {
    const overlay = document.getElementById('search-overlay');
    overlay.style.display = 'none';
    selectedResultIndex = -1;
}

// Show search hint
function showSearchHint() {
    const resultsContainer = document.getElementById('search-results');
    resultsContainer.innerHTML = '<div class="search-hint">Start typing to search...</div>';
}

// Perform search
function performSearch(query) {
    if (!searchIndex || !query.trim()) {
        showSearchHint();
        return;
    }

    const results = searchIndex.search(query, {
        combineWith: 'AND',
        prefix: true
    });

    displayResults(results.slice(0, 20), query); // Show top 20 results
}

// Display search results
function displayResults(results, query) {
    const resultsContainer = document.getElementById('search-results');
    selectedResultIndex = -1;

    if (results.length === 0) {
        resultsContainer.innerHTML = '<div class="search-no-results">No results found</div>';
        return;
    }

    const html = results.map((result, index) => {
        const doc = searchData[result.id];
        const highlightedTitle = highlightText(doc.title, query);
        const highlightedBlurb = highlightText(doc.blurb, query);

        return `
            <div class="search-result" data-index="${index}" data-url="${doc.url}">
                <div class="search-result-title">${highlightedTitle}</div>
                ${doc.blurb ? `<div class="search-result-blurb">${highlightedBlurb}</div>` : ''}
                <div class="search-result-meta">
                    ${doc.category ? `<span class="search-result-category">${doc.category}</span>` : ''}
                </div>
            </div>
        `;
    }).join('');

    resultsContainer.innerHTML = html;

    // Add click handlers
    resultsContainer.querySelectorAll('.search-result').forEach(el => {
        el.addEventListener('click', () => {
            const url = el.dataset.url;
            // TODO: support file:// (doc.url is absolute)
            window.location.href = url;
        });
    });
}

// Highlight search terms in text
function highlightText(text, query) {
    if (!text) return '';

    const terms = query.toLowerCase().split(/\s+/).filter(t => t.length > 0);
    let result = text;

    terms.forEach(term => {
        const regex = new RegExp(`(${escapeRegex(term)})`, 'gi');
        result = result.replace(regex, '<mark>$1</mark>');
    });

    return result;
}

// Escape regex special characters
function escapeRegex(str) {
    return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

// Navigate results with keyboard
function navigateResults(direction) {
    const results = document.querySelectorAll('.search-result');
    if (results.length === 0) return;

    // Remove previous selection
    if (selectedResultIndex >= 0 && selectedResultIndex < results.length) {
        results[selectedResultIndex].classList.remove('selected');
    }

    // Update index
    if (direction === 'down') {
        selectedResultIndex = (selectedResultIndex + 1) % results.length;
    } else if (direction === 'up') {
        selectedResultIndex = selectedResultIndex <= 0 ? results.length - 1 : selectedResultIndex - 1;
    }

    // Add new selection
    if (selectedResultIndex >= 0 && selectedResultIndex < results.length) {
        const selected = results[selectedResultIndex];
        selected.classList.add('selected');
        selected.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    }
}

// Select current result
function selectResult() {
    const results = document.querySelectorAll('.search-result');
    if (selectedResultIndex >= 0 && selectedResultIndex < results.length) {
        const url = results[selectedResultIndex].dataset.url;
        // TODO: support file:// (doc.url is absolute)
        window.location.href = url;
    }
}

// Event listeners
document.addEventListener('DOMContentLoaded', () => {
    const searchInput = document.getElementById('search-input');
    const searchOverlay = document.getElementById('search-overlay');

    // Search input handler
    if (searchInput) {
        searchInput.addEventListener('input', (e) => {
            performSearch(e.target.value);
        });
    }

    // Keyboard shortcuts
    document.addEventListener('keydown', (e) => {
        // Cmd+K or Ctrl+K to open search
        if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
            e.preventDefault();
            openSearch();
            return;
        }

        // Only handle other shortcuts when search is open
        if (searchOverlay && searchOverlay.style.display === 'flex') {
            if (e.key === 'Escape') {
                e.preventDefault();
                closeSearch();
            } else if (e.key === 'ArrowDown') {
                e.preventDefault();
                navigateResults('down');
            } else if (e.key === 'ArrowUp') {
                e.preventDefault();
                navigateResults('up');
            } else if (e.key === 'Enter') {
                e.preventDefault();
                selectResult();
            }
        }
    });

    // Close on overlay click
    if (searchOverlay) {
        searchOverlay.addEventListener('click', (e) => {
            if (e.target === searchOverlay) {
                closeSearch();
            }
        });
    }
});
