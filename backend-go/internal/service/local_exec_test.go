package service

import (
	"os"
	"path/filepath"
	"testing"
)

// 构造临时项目目录：根目录无清单，frontend/ 含 package.json，backend-go/ 含 go.mod 与 cmd/server/main.go
func setupProjectTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	mk := func(rel string, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mk("frontend/package.json", `{"name":"x","scripts":{"build":"true"}}`)
	mk("backend-go/go.mod", "module example.com/x\n\ngo 1.21\n")
	mk("backend-go/cmd/server/main.go", "package main\nfunc main() {}\n")
	return root
}

func TestDetectFrontendDir(t *testing.T) {
	root := setupProjectTree(t)
	dir, err := DetectFrontendDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(root, "frontend") {
		t.Fatalf("期望 frontend 子目录, 得到 %s", dir)
	}

	// 根目录含 package.json 时优先返回根目录
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir, err = DetectFrontendDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if dir != root {
		t.Fatalf("期望根目录, 得到 %s", dir)
	}
}

func TestDetectBackendDir(t *testing.T) {
	root := setupProjectTree(t)
	dir, err := DetectBackendDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(root, "backend-go") {
		t.Fatalf("期望 backend-go 子目录, 得到 %s", dir)
	}
}

func TestDetectGoMainPackage_CmdServer(t *testing.T) {
	root := setupProjectTree(t)
	pkg, err := DetectGoMainPackage(filepath.Join(root, "backend-go"))
	if err != nil {
		t.Fatal(err)
	}
	if pkg != "./cmd/server" {
		t.Fatalf("期望 ./cmd/server, 得到 %s", pkg)
	}
}

func TestDetectFrontendDir_NotFound(t *testing.T) {
	root := t.TempDir()
	if _, err := DetectFrontendDir(root); err == nil {
		t.Fatal("期望报错, 实际成功")
	}
}
