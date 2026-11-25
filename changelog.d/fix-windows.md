### Fixed

- `ai-specview` builds and runs on Windows 10 and later: `go install` no
  longer fails there, and the daemon, the MCP bridge and the hook work with
  Windows processes and paths.
- The plugin finds the binary on Windows when it is installed under a
  `GOPATH` such as `C:\Users\you\go`.
