package md2html

import (
	"fmt"
	"net/http"
	"path"
	"strings"
)

// normalizeBasePath cleans a -base value into either "" or a rooted path
// with no trailing slash, such as "/docs".
func normalizeBasePath(base string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" || base == "/" {
		return "", nil
	}
	if strings.Contains(base, "://") {
		return "", fmt.Errorf("invalid base path %q: want a path such as /docs, not a URL", base)
	}
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	base = path.Clean(base)
	if base == "/" {
		return "", nil
	}
	return base, nil
}

// mountAt serves content under base. Requests outside base redirect into
// it, so a link written for the published site resolves in preview even
// when the server was started inside the subtree it names.
//
// The endpoints the pages themselves call, live reload in particular, stay
// reachable at the root as well: their URLs are absolute in the rendered
// HTML and do not know about base.
func mountAt(base string, content *http.ServeMux, register func(mux *http.ServeMux)) http.Handler {
	if base == "" {
		return content
	}
	mux := http.NewServeMux()
	register(mux)
	mux.Handle(base+"/", http.StripPrefix(base, content))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		target := base + r.URL.Path
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusTemporaryRedirect)
	})
	return mux
}
