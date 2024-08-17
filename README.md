# azconsoles

## Description

This package provides Azure consoles (serial and cloud) functionality for use in other golang projects.

## Installation

```bash
go get -u github.com/brendank310/azconsoles
```

## Usage

Examples of how to use this package can be found in the `cmd/azconsoles/` directory.

```bash
SUBSCRIPTION_ID=<subscription-id> RESOURCE_GROUP=<resource-group> VM_NAME=<vm-name> go run cmd/azconsoles/sericon.go
```

```bash
go run cmd/azconsoles/cloudshell.go
```

## License

Apache 2.0
