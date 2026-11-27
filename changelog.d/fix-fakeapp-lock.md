### Fixed

- `ai-contextview fakeapp` no longer breaks a running daemon. Before, it
  started on the daemon's socket, even with `--help`, and deleted the socket
  file, so bridges, `serve --stop` and `serve --restart` could no longer reach
  the daemon. Now `--help` prints the usage, other arguments are an error, and
  `fakeapp` refuses to start while a daemon runs.
