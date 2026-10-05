package md2html

import "testing"

func TestPageLayoutDirs(t *testing.T) {
	l := pageLayout{ext: ".html", index: "README.md", dirs: true}
	tests := []struct {
		source string
		file   string
		url    string
		root   string
	}{
		{"README.md", "index.html", "./", "./"},
		{"index.md", "index.html", "./", "./"},
		{"guide.md", "guide/index.html", "guide/", "../"},
		{"api/index.md", "api/index.html", "api/", "../"},
		{"api/client.mdx", "api/client/index.html", "api/client/", "../../"},
		{"api/README.md", "api/README/index.html", "api/README/", "../../"},
	}
	for _, tt := range tests {
		if got := l.file(tt.source); got != tt.file {
			t.Errorf("file(%q) = %q, want %q", tt.source, got, tt.file)
		}
		if got := l.url(tt.source); got != tt.url {
			t.Errorf("url(%q) = %q, want %q", tt.source, got, tt.url)
		}
		if got := l.root(tt.source); got != tt.root {
			t.Errorf("root(%q) = %q, want %q", tt.source, got, tt.root)
		}
	}
}

func TestPageLayoutLink(t *testing.T) {
	tests := []struct {
		current, target string
		file, dirs      string
	}{
		{"README.md", "guide.md", "guide.html", "guide/"},
		{"guide.md", "README.md", "index.html", "../"},
		{"guide.md", "guide.md", "guide.html", "./"},
		{"guide.md", "api/client.md", "api/client.html", "../api/client/"},
		{"api/client.md", "api/index.md", "index.html", "../"},
		{"api/index.md", "api/client.md", "client.html", "client/"},
		{"api/client.md", "guide.md", "../guide.html", "../../guide/"},
		{"README.md", "README.md", "index.html", "./"},
	}
	for _, tt := range tests {
		l := pageLayout{ext: ".html", index: "README.md"}
		if got := l.link(tt.current, tt.target); got != tt.file {
			t.Errorf("file layout: link(%q, %q) = %q, want %q", tt.current, tt.target, got, tt.file)
		}
		l.dirs = true
		if got := l.link(tt.current, tt.target); got != tt.dirs {
			t.Errorf("dir layout: link(%q, %q) = %q, want %q", tt.current, tt.target, got, tt.dirs)
		}
	}
}

func TestPageLayoutRelocate(t *testing.T) {
	l := pageLayout{ext: ".html", dirs: true}
	tests := []struct {
		source, ref string
		want        string
		ok          bool
	}{
		{"guide.md", "logo.png", "../logo.png", true},
		{"guide.md", "../up.png", "../../up.png", true},
		{"guide.md", "#top", "#top", false},
		{"guide.md", "/abs.png", "/abs.png", false},
		{"guide.md", "https://example.com/a.png", "https://example.com/a.png", false},
		{"guide.md", "data:image/png;base64,AA", "data:image/png;base64,AA", false},
		{"index.md", "logo.png", "logo.png", false},
		{"api/index.md", "diagram.png", "diagram.png", false},
	}
	for _, tt := range tests {
		got, ok := l.relocate(tt.source, tt.ref)
		if got != tt.want || ok != tt.ok {
			t.Errorf("relocate(%q, %q) = %q, %v, want %q, %v", tt.source, tt.ref, got, ok, tt.want, tt.ok)
		}
	}
	l.dirs = false
	if got, ok := l.relocate("guide.md", "logo.png"); ok {
		t.Errorf("file layout: relocate changed logo.png to %q", got)
	}
}
