package handler

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func touchFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetectBuildSteps(t *testing.T) {
	cases := []struct {
		name  string
		setup func(dir string)
		want  []string
	}{
		{
			name: "根目录 go.mod",
			setup: func(dir string) {
				touchFile(t, filepath.Join(dir, "go.mod"))
			},
			want: []string{"go build ./..."},
		},
		{
			name: "根目录 package.json",
			setup: func(dir string) {
				touchFile(t, filepath.Join(dir, "package.json"))
			},
			want: []string{"npm run build"},
		},
		{
			name: "monorepo 一级子目录 backend-go + frontend",
			setup: func(dir string) {
				touchFile(t, filepath.Join(dir, "backend-go", "go.mod"))
				touchFile(t, filepath.Join(dir, "frontend", "package.json"))
				touchFile(t, filepath.Join(dir, "Makefile"))
			},
			want: []string{"(cd backend-go && go build ./...)", "(cd frontend && npm run build)"},
		},
		{
			name: "忽略隐藏目录与 node_modules",
			setup: func(dir string) {
				touchFile(t, filepath.Join(dir, ".git", "go.mod"))
				touchFile(t, filepath.Join(dir, "node_modules", "package.json"))
			},
			want: nil,
		},
		{
			name:  "空目录返回空",
			setup: func(dir string) {},
			want:  nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.setup(dir)
			got := detectBuildSteps(dir)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("detectBuildSteps() = %v, want %v", got, tc.want)
			}
		})
	}
}
