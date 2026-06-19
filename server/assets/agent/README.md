GoBup agent assets live here for embedded controller distribution.

Release builds copy `scripts/install_agent.sh` and packaged Rust agent archives
into this directory before building the Go server with the `embed` tag. When an
archive is not embedded, the controller redirects the request to GitHub Releases.
