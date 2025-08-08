# azconsoles

## Description

This package provides Azure consoles (serial and cloud) functionality for use in other golang projects.

## Installation

```bash
go get -u github.com/brendank310/azconsoles
```

## Tools

### sericonssh - SSH over Azure Serial Console

A drop-in CLI that feels like `ssh` but transports over Azure Serial Console using PPP.

**Usage:**
```bash
sericonssh <user>@<vm>.<resource_group>.<subscription_id> [ssh args...]
```

**Examples:**
```bash
# Basic connection
sericonssh user@myvm.myrg.12345678-1234-1234-1234-123456789012

# With SSH options
sericonssh user@myvm.myrg.sub -- -v -o StrictHostKeyChecking=no

# With custom PPP settings
sericonssh --ppp-local 10.8.0.2 --ppp-peer 10.8.0.1 --ssh-port 22 user@myvm.myrg.sub

# Dry run to see execution plan
sericonssh --dry-run user@myvm.myrg.sub
```

**Prerequisites:**
- `pppd` must be installed (`apt install ppp` on Ubuntu/Debian, `yum install ppp` on RHEL/CentOS)
- Azure authentication (environment variables or `az login`)
- VM must have PPP-enabled SSH daemon on the specified port (default 2222)

### Other Tools

Examples of how to use this package can be found in the `cmd/azconsoles/` directory.

```bash
SUBSCRIPTION_ID=<subscription-id> RESOURCE_GROUP=<resource-group> VM_NAME=<vm-name> go run cmd/azconsoles/sericon.go
```

```bash
go run cmd/azconsoles/cloudshell.go
```

## License

Apache 2.0
