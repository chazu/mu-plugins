// #PluginConfig: remote-file plugin manifest.
package mu

// Go plugin: the entrypoint is the compiled binary, which is not committed.
// `mu plugin install` builds it from the catalog build command, run in this
// directory:
//
//   go build -trimpath -o plugin .
//
// No toolchain: mu executes the binary directly. The source is vendored from
// github.com/chazu/mu/plugins/remote-file, which owns its tests.
plugin: {
	entrypoint: "plugin"
	files: ["plugin", "GUIDE.md"]
	guide: "GUIDE.md"
}
