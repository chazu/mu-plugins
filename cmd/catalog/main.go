// Command catalog generates the immutable source catalog consumed by mu's
// future GitHub-backed plugin installer. It also creates deterministic source
// archives whose hashes are recorded in the catalog and uploaded to a release.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type catalogSource struct {
	SchemaVersion int            `json:"schema_version"`
	Repository    string         `json:"repository"`
	ReleaseTag    string         `json:"release_tag"`
	Plugins       []sourcePlugin `json:"plugins"`
}

type sourcePlugin struct {
	Name         string        `json:"name"`
	Version      string        `json:"version"`
	Description  string        `json:"description"`
	Entrypoint   string        `json:"entrypoint"`
	Toolchain    string        `json:"toolchain,omitempty"`
	Requirements []string      `json:"requirements,omitempty"`
	Schemas      []schemaRef   `json:"schemas,omitempty"`
	PudlMappings []pudlMapping `json:"pudl_mappings,omitempty"`
	Build        *buildSpec    `json:"build,omitempty"`
}

type schemaRef struct {
	Module  string `json:"module"`
	Version string `json:"version"`
	Path    string `json:"path"`
}

type pudlMapping struct {
	ResourceType string `json:"resource_type"`
	Schema       string `json:"schema"`
}

type buildSpec struct {
	Command []string `json:"command"`
	Sources []string `json:"sources"`
}

type catalog struct {
	SchemaVersion int             `json:"schema_version"`
	Repository    string          `json:"repository"`
	ReleaseTag    string          `json:"release_tag"`
	Plugins       []catalogPlugin `json:"plugins"`
}

type catalogPlugin struct {
	Name         string        `json:"name"`
	Version      string        `json:"version"`
	AssetURL     string        `json:"asset_url"`
	SHA256       string        `json:"sha256"`
	Path         string        `json:"path"`
	Entrypoint   string        `json:"entrypoint"`
	Toolchain    string        `json:"toolchain,omitempty"`
	Requirements []string      `json:"requirements"`
	Schemas      []schemaRef   `json:"schemas"`
	PudlMappings []pudlMapping `json:"pudl_mappings"`
	Build        *buildSpec    `json:"build,omitempty"`
}

var (
	entrypointRE = regexp.MustCompile(`(?m)\bentrypoint:\s*"([^"]+)"`)
	toolchainRE  = regexp.MustCompile(`(?m)\btoolchain:\s*"([^"]+)"`)
	bbVersionRE  = regexp.MustCompile(`(?m)"version"\s+"([^"]+)"`)
	goVersionRE  = regexp.MustCompile(`(?m)SecretPlugin\s*\(\s*"[^"]+"\s*,\s*"([^"]+)"`)
)

func main() {
	var (
		source      = flag.String("source", "catalog.source.json", "catalog source metadata")
		output      = flag.String("output", "catalog.json", "generated catalog path, or - for stdout")
		assetsDir   = flag.String("assets-dir", "", "directory for deterministic plugin archives")
		repository  = flag.String("repository", "", "GitHub repository owner/name; defaults to source metadata")
		releaseTag  = flag.String("release-tag", "", "GitHub release tag; defaults to source metadata")
		packageRoot = flag.String("package-root", "plugins", "plugin package directory")
	)
	flag.Parse()

	if err := generate(*source, *output, *assetsDir, *repository, *releaseTag, *packageRoot); err != nil {
		fmt.Fprintf(os.Stderr, "catalog: %v\n", err)
		os.Exit(1)
	}
}

func generate(sourcePath, outputPath, assetsDir, repository, releaseTag, packageRoot string) error {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}
	var src catalogSource
	if err := json.Unmarshal(data, &src); err != nil {
		return fmt.Errorf("decode source: %w", err)
	}
	if src.SchemaVersion != 1 {
		return fmt.Errorf("unsupported schema_version %d", src.SchemaVersion)
	}
	if repository != "" {
		src.Repository = repository
	}
	if releaseTag != "" {
		src.ReleaseTag = releaseTag
	}
	if src.Repository == "" || src.ReleaseTag == "" {
		return fmt.Errorf("repository and release_tag are required")
	}
	if strings.ContainsAny(src.ReleaseTag, "/\\") {
		return fmt.Errorf("release_tag %q may not contain path separators", src.ReleaseTag)
	}

	if err := validatePackageSet(packageRoot, src.Plugins); err != nil {
		return err
	}
	plugins := make([]catalogPlugin, 0, len(src.Plugins))
	for _, p := range src.Plugins {
		packageDir := filepath.Join(packageRoot, p.Name)
		if err := validatePackage(packageDir, p); err != nil {
			return fmt.Errorf("plugin %s: %w", p.Name, err)
		}

		sha := ""
		if assetsDir != "" {
			sha, err = archivePackage(packageDir, p.Name, p.Version, assetsDir)
			if err != nil {
				return fmt.Errorf("plugin %s: archive: %w", p.Name, err)
			}
		}
		plugins = append(plugins, catalogPlugin{
			Name:         p.Name,
			Version:      p.Version,
			AssetURL:     fmt.Sprintf("https://github.com/%s/releases/download/%s/%s-%s.tar.gz", src.Repository, src.ReleaseTag, p.Name, p.Version),
			SHA256:       sha,
			Path:         filepath.ToSlash(filepath.Join(packageRoot, p.Name)),
			Entrypoint:   p.Entrypoint,
			Toolchain:    p.Toolchain,
			Requirements: nonNilStrings(p.Requirements),
			Schemas:      nonNilSchemas(p.Schemas),
			PudlMappings: nonNilMappings(p.PudlMappings),
			Build:        p.Build,
		})
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].Name < plugins[j].Name })

	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(catalog{SchemaVersion: src.SchemaVersion, Repository: src.Repository, ReleaseTag: src.ReleaseTag, Plugins: plugins}); err != nil {
		return fmt.Errorf("encode catalog: %w", err)
	}
	if outputPath == "-" {
		_, err = os.Stdout.Write(out.Bytes())
		return err
	}
	if err := os.WriteFile(outputPath, out.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write catalog: %w", err)
	}
	return nil
}

func validatePackageSet(root string, plugins []sourcePlugin) error {
	seen := make(map[string]bool, len(plugins))
	for _, p := range plugins {
		if p.Name == "" || p.Version == "" || p.Entrypoint == "" {
			return fmt.Errorf("each plugin requires name, version, and entrypoint")
		}
		if seen[p.Name] {
			return fmt.Errorf("duplicate plugin %q", p.Name)
		}
		seen[p.Name] = true
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read package root: %w", err)
	}
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			dirs = append(dirs, entry.Name())
		}
	}
	sort.Strings(dirs)
	if len(dirs) != len(plugins) {
		return fmt.Errorf("source lists %d plugins but %s contains %d directories", len(plugins), root, len(dirs))
	}
	for _, dir := range dirs {
		if !seen[dir] {
			return fmt.Errorf("package directory %q is missing from source metadata", dir)
		}
	}
	return nil
}

func validatePackage(packageDir string, p sourcePlugin) error {
	manifestPath := filepath.Join(packageDir, "mu.cue")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	entryMatch := entrypointRE.FindSubmatch(manifest)
	if len(entryMatch) != 2 || string(entryMatch[1]) != p.Entrypoint {
		return fmt.Errorf("catalog entrypoint %q does not match mu.cue", p.Entrypoint)
	}
	toolMatch := toolchainRE.FindSubmatch(manifest)
	manifestToolchain := ""
	if len(toolMatch) == 2 {
		manifestToolchain = string(toolMatch[1])
	}
	if p.Toolchain != "" && manifestToolchain != p.Toolchain {
		return fmt.Errorf("catalog toolchain %q does not match mu.cue %q", p.Toolchain, manifestToolchain)
	}
	if _, err := os.Stat(filepath.Join(packageDir, "GUIDE.md")); err != nil {
		return fmt.Errorf("GUIDE.md: %w", err)
	}
	entrypointPath := filepath.Join(packageDir, p.Entrypoint)
	if _, err := os.Stat(entrypointPath); err != nil {
		if p.Build == nil {
			return fmt.Errorf("entrypoint %q is missing and has no build specification", p.Entrypoint)
		}
		for _, source := range p.Build.Sources {
			if _, sourceErr := os.Stat(filepath.Join(packageDir, source)); sourceErr != nil {
				return fmt.Errorf("build source %q: %w", source, sourceErr)
			}
		}
	}
	version, err := discoverVersion(packageDir)
	if err != nil {
		return err
	}
	if version != p.Version {
		return fmt.Errorf("catalog version %q does not match discover version %q", p.Version, version)
	}
	for _, schema := range p.Schemas {
		if _, err := os.Stat(filepath.Join(packageDir, schema.Path)); err != nil {
			return fmt.Errorf("schema path %q: %w", schema.Path, err)
		}
		for _, value := range []string{schema.Module, schema.Version, schema.Path} {
			if !bytes.Contains(manifest, []byte(value)) {
				return fmt.Errorf("schema declaration %q is missing from mu.cue", value)
			}
		}
	}
	if len(p.PudlMappings) > 0 {
		if _, err := os.Stat(filepath.Join(packageDir, "pudl.cue")); err != nil {
			return fmt.Errorf("pudl.cue: %w", err)
		}
	}
	return nil
}

func discoverVersion(packageDir string) (string, error) {
	if data, err := os.ReadFile(filepath.Join(packageDir, "plugin.bb")); err == nil {
		match := bbVersionRE.FindSubmatch(data)
		if len(match) == 2 {
			return string(match[1]), nil
		}
	}
	if data, err := os.ReadFile(filepath.Join(packageDir, "main.go")); err == nil {
		match := goVersionRE.FindSubmatch(data)
		if len(match) == 2 {
			return string(match[1]), nil
		}
	}
	return "", fmt.Errorf("could not find discover version in plugin source")
}

func archivePackage(packageDir, name, version, assetsDir string) (string, error) {
	var files []string
	err := filepath.WalkDir(packageDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)

	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	gz.Header.ModTime = time.Unix(0, 0)
	gz.Header.OS = 3
	tw := tar.NewWriter(gz)
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("unsupported non-regular file %s", path)
		}
		rel, err := filepath.Rel(packageDir, path)
		if err != nil {
			return "", err
		}
		header := &tar.Header{
			Name:     filepath.ToSlash(filepath.Join("plugins", name, rel)),
			Mode:     int64(info.Mode().Perm()),
			Size:     info.Size(),
			ModTime:  time.Unix(0, 0),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(header); err != nil {
			return "", err
		}
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(tw, file)
		closeErr := file.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	if err := tw.Close(); err != nil {
		return "", err
	}
	if err := gz.Close(); err != nil {
		return "", err
	}

	hash := sha256.Sum256(archive.Bytes())
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return "", err
	}
	assetPath := filepath.Join(assetsDir, fmt.Sprintf("%s-%s.tar.gz", name, version))
	if err := os.WriteFile(assetPath, archive.Bytes(), 0o644); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash[:]), nil
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func nonNilSchemas(values []schemaRef) []schemaRef {
	if values == nil {
		return []schemaRef{}
	}
	return values
}

func nonNilMappings(values []pudlMapping) []pudlMapping {
	if values == nil {
		return []pudlMapping{}
	}
	return values
}
