# mrcee

(non) Malevolent Remote Code Execution Engine a.k.a. `mrcee` is a job worker service that provides an API to run arbitrary Linux processes. These processes can be any executable program that is available on the machine running the service.

## Installation

TBD

## Usage Examples
```bash
# start a job
mrcee start -- echo "Hello World"

# start a job and stream its output
mrcee start -f -- tail -f error.log

# get job status
mrcee status acde070d-8c4c-4f0d-9d8a-162843c10333

# cancel job
mrcee stop acde070d-8c4c-4f0d-9d8a-162843c10333

# list jobs
mrcee ps

# get job's output; if job still running, gets output up to this point in time
mrcee get acde070d-8c4c-4f0d-9d8a-162843c10333

# stream job's output
mrcee get -f acde070d-8c4c-4f0d-9d8a-162843c10333
```

## Design

See [DESIGN.md](docs/DESIGN.md)
