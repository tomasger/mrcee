# mrcee

(non) Malevolent Remote Code Execution Engine a.k.a. `mrcee` is a job worker service that provides an API to run arbitrary Linux processes. These processes can be any executable program that is available on the machine running the service.

## Installation

TBD

## Usage Examples

### Starting a job
```bash
$ mrcee start -- echo "Hello World"
Job started: acde070d-8c4c-4f0d-9d8a-162843c10333
```
### Querying job status
```bash
# Job that finished
$ mrcee status acde070d-8c4c-4f0d-9d8a-162843c10333
Job ID:      acde070d-8c4c-4f0d-9d8a-162843c10333
State:       EXITED
Started:     2026-10-21 13:00:00
Finished:    2026-10-21 13:00:00
Duration:    12ms
Exit code:   0
# Job that's still running
$ mrcee status acde070d-8c4c-4f0d-9d8a-162843c10334
Job ID:      acde070d-8c4c-4f0d-9d8a-162843c10334
State:       RUNNING
Started:     2026-10-21 13:00:00
Duration:    16s
```
### Retrieving output
```bash
$ mrcee get acde070d-8c4c-4f0d-9d8a-162843c10333
Hello World
# can also save it to file
$ mrcee get acde070d-8c4c-4f0d-9d8a-162843c10333 > echo.out
$ cat echo.out
Hello World
```
### Querying a failed job
```bash
$ mrcee start -- sh -c 'echo "Something failed" >&2; exit 42'
Job started: 535c904e-e8b6-4b37-babc-bfe5008cf481

$ mrcee status 535c904e-e8b6-4b37-babc-bfe5008cf481
Job ID:      535c904e-e8b6-4b37-babc-bfe5008cf481
State:       EXITED
Started:     2026-10-21 13:10:00
Finished:    2026-10-21 13:10:00
Duration:    7ms
Exit code:   42

$ mrcee get d86a6e8b-6243-471e-a96b-1a26ce365ff7
Something failed
```
### Stopping a running job
```bash
$ mrcee start -- sleep 120
Job started: acde070d-8c4c-4f0d-9d8a-162843c10333

$ mrcee stop acde070d-8c4c-4f0d-9d8a-162843c10333
Job ID:      acde070d-8c4c-4f0d-9d8a-162843c10333
State:       STOPPED
Started:     2026-10-21 13:05:00
Finished:    2026-10-21 13:05:08
Duration:    8s
Signal:      SIGKILL (9)
```


## Design

See [DESIGN.md](docs/DESIGN.md)
