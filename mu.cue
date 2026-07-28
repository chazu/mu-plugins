// Root project for the official mu plugin catalog.
//
// Each directory under plugins/ is a self-contained plugin package. The
// per-plugin mu.cue files are merged when mu loads this project, giving CI a
// single project root while keeping packages independently installable.
package mu

// The default package set uses Babashka for its executable plugins. Keep the
// toolchain declaration here so the catalog can be tested and built from one
// project root, without depending on a caller's local mu.cue.
toolchains: [{
	toolchain: "bb"
	from:      "scratch"
	config: {
		version: "1.12.216"
		url:     "https://github.com/babashka/babashka/releases/download/v1.12.216/babashka-1.12.216-macos-aarch64.tar.gz"
		sha256:  "91499b3f430038f9b40e433215256a6e5392942780dca9984d493d2bcca7055d"
	}
}]
