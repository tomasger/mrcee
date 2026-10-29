# mrcee

(non) Malevolent Remote Code Execution Engine a.k.a. `mrcee` is a job worker service that provides an API to run arbitrary Linux processes. These processes can be any executable program that is available on the machine running the service.

## Installation

Target OS: 64-bit Linux
Dependencies: Go 1.27.2 or later

### Certificates

`mrcee` server and client require a mutual TLS connection to work. `cmd/certgen` is a helper script to generate certificates for development builds. Which are placed in `.local/certs/` by default.

```sh
go run cmd/certgen/main.go
```

Use `-o` to change output location and `-role viewer` to generate client with a read only viewer role.

Server and client will fetch certificates from the default location, but those can also be overridden with flags.

### CLI

CLI can be installed as an executable:
```sh
make mrcee-install
mrcee --help # if you have $GOPATH/bin in your PATH
```

Or simply ran directly:
```sh
go run ./cmd/mrcee --help
```

### Server
Compile the server and move the executable to your path of choice:
```sh
make server && sudo mv mrcee-server /usr/local/bin
```

Or run directly:
```sh
go run ./cmd/server
```

## Usage Examples

See [Usage Exampes](docs/DESIGN.md#cli-ux)

## Design

See [DESIGN.md](docs/DESIGN.md)
