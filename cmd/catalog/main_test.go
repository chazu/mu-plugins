package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchivePackageIsDeterministic(t *testing.T) {
	root := t.TempDir()
	packageDir := filepath.Join(root, "plugins", "demo")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "plugin.bb"), []byte("hello\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "GUIDE.md"), []byte("guide\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	firstDir := filepath.Join(root, "first")
	secondDir := filepath.Join(root, "second")
	firstHash, err := archivePackage(packageDir, "demo", "0.1.0", firstDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := archivePackage(packageDir, "demo", "0.1.0", secondDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Fatalf("archive hashes differ: %s != %s", firstHash, secondHash)
	}
	first, err := os.ReadFile(filepath.Join(firstDir, "demo-0.1.0.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(secondDir, "demo-0.1.0.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("deterministic archives differ byte-for-byte")
	}
}

func TestGenerateValidatesAndWritesCatalog(t *testing.T) {
	root := t.TempDir()
	packageRoot := filepath.Join(root, "plugins")
	packageDir := filepath.Join(packageRoot, "demo")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"mu.cue":    []byte("package mu\nplugin: {entrypoint: \"plugin.bb\", toolchain: \"bb\"}\n"),
		"GUIDE.md":  []byte("guide\n"),
		"plugin.bb": []byte("{\"version\" \"0.1.0\"}\n"),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(packageDir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	source := filepath.Join(root, "catalog.source.json")
	sourceData := []byte(`{
  "schema_version": 1,
  "repository": "example/plugins",
  "release_tag": "catalog-v0.1.0",
  "plugins": [{
    "name": "demo",
    "version": "0.1.0",
    "description": "demo",
    "entrypoint": "plugin.bb",
    "toolchain": "bb"
  }]
}`)
	if err := os.WriteFile(source, sourceData, 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "catalog.json")
	if err := generate(source, out, filepath.Join(root, "assets"), "", "", packageRoot); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var got catalog
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Plugins) != 1 || got.Plugins[0].SHA256 == "" {
		t.Fatalf("unexpected generated catalog: %+v", got)
	}
}

func TestGenerateGoPluginExcludesBuildOutput(t *testing.T) {
	root := t.TempDir()
	packageRoot := filepath.Join(root, "plugins")
	packageDir := filepath.Join(packageRoot, "demo")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"mu.cue":   []byte("package mu\nplugin: {entrypoint: \"plugin\"}\n"),
		"GUIDE.md": []byte("guide\n"),
		"go.mod":   []byte("module example.com/demo\n"),
		"main.go":  []byte("package main\n\nvar p = Plugin{\n\tName:    \"demo\",\n\tVersion: \"0.2.0\",\n}\n"),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(packageDir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	source := filepath.Join(root, "catalog.source.json")
	sourceData := []byte(`{
  "schema_version": 1,
  "repository": "example/plugins",
  "release_tag": "catalog-v0.1.0",
  "plugins": [{
    "name": "demo",
    "version": "0.2.0",
    "entrypoint": "plugin",
    "build": {"command": ["go", "build", "-trimpath", "-o", "plugin", "."], "sources": ["main.go", "go.mod"]}
  }]
}`)
	if err := os.WriteFile(source, sourceData, 0o644); err != nil {
		t.Fatal(err)
	}

	cleanAssets := filepath.Join(root, "clean")
	if err := generate(source, filepath.Join(root, "clean.json"), cleanAssets, "", "", packageRoot); err != nil {
		t.Fatal(err)
	}
	// A locally built binary must neither ship nor change the archive.
	if err := os.WriteFile(filepath.Join(packageDir, "plugin"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	builtAssets := filepath.Join(root, "built")
	if err := generate(source, filepath.Join(root, "built.json"), builtAssets, "", "", packageRoot); err != nil {
		t.Fatal(err)
	}
	clean, err := os.ReadFile(filepath.Join(cleanAssets, "demo-0.2.0.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	built, err := os.ReadFile(filepath.Join(builtAssets, "demo-0.2.0.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(clean, built) {
		t.Fatal("built entrypoint changed the source archive")
	}
	gz, err := gzip.NewReader(bytes.NewReader(built))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Name == "plugins/demo/plugin" {
			t.Fatal("archive contains the build output")
		}
	}
}

func TestGenerateRejectsRemovedSourceFields(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "catalog.source.json")
	sourceData := []byte(`{
  "schema_version": 1,
  "repository": "example/plugins",
  "release_tag": "catalog-v0.1.0",
  "plugins": [{
    "name": "demo",
    "version": "0.1.0",
    "entrypoint": "plugin.bb",
    "schemas": [{"module": "mu/demo", "version": "v1", "path": "schemas"}]
  }]
}`)
	if err := os.WriteFile(source, sourceData, 0o644); err != nil {
		t.Fatal(err)
	}
	err := generate(source, filepath.Join(root, "catalog.json"), "", "", "", filepath.Join(root, "plugins"))
	if err == nil || !strings.Contains(err.Error(), `unknown field "schemas"`) {
		t.Fatalf("generate error = %v, want unknown field \"schemas\"", err)
	}
}
