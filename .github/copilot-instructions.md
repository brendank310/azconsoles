# azconsoles

Azure consoles (serial and cloud shell) Go library and CLI tools for connecting to Azure VM serial consoles and Cloud Shell.

**Always reference these instructions first and fallback to search or bash commands only when you encounter unexpected information that does not match the info here.**

## Working Effectively

### Bootstrap and Build
- Install Go 1.19+ (tested with Go 1.24.5): `go version`
- Download dependencies: `go mod tidy` -- takes 10-15 seconds on first run
- Build all binaries: `make all` -- takes 1-25 seconds, NEVER CANCEL, set timeout to 60+ seconds
- Clean build artifacts: `make clean`

### Code Quality and Validation
- Format code: `go fmt ./...` -- ALWAYS run before committing
- Vet packages: `go vet ./pkg/...` -- takes <2 seconds
- Test packages: `go test ./pkg/...` -- takes <1 second (no actual tests exist)
- DO NOT run `go test ./...` -- this fails due to multiple main functions in cmd/azconsoles/

### Authentication Setup
**CRITICAL**: All CLI applications require Azure authentication to function. Without proper auth, they will panic with DefaultAzureCredential errors (this is expected behavior).

Required setup for actual usage:
- Install Azure CLI: `curl -sL https://aka.ms/InstallAzureCLIDeb | sudo bash`
- Login to Azure: `az login`
- Set environment variables for sericon tools:
  - `SUBSCRIPTION_ID=<your-subscription-id>`
  - `RESOURCE_GROUP=<your-resource-group>`
  - `VM_NAME=<your-vm-name>`

### Run Applications
- Cloud Shell client: `./bin/cloudshell`
- Basic serial console: `SUBSCRIPTION_ID=sub RESOURCE_GROUP=rg VM_NAME=vm ./bin/sericon`
- PTY serial console: `SUBSCRIPTION_ID=sub RESOURCE_GROUP=rg VM_NAME=vm ./bin/sericon-pty`
- PPP network setup (for serial console networking): `sudo scripts/ppp-client-setup.sh <pts-number>`

**Note**: Applications will panic without valid Azure credentials (expected behavior for testing).

## Validation

### Build Validation
Always validate that your changes don't break the build:
```bash
go mod tidy
go fmt ./...
go vet ./pkg/...
make clean && make all
```

### Functional Validation
Since the applications require Azure resources, validation scenarios are:
1. **Build Success**: All three binaries (cloudshell, sericon, sericon-pty) build successfully
2. **Graceful Auth Failure**: Applications show proper DefaultAzureCredential error messages when run without auth
3. **Code Quality**: No formatting issues (`go fmt` produces no output), no vet warnings
4. **Dependencies**: `go mod tidy` completes without errors

### Manual Testing Requirements
- ALWAYS run complete build validation after making changes
- Test that binaries execute and fail gracefully without Azure auth
- Verify that environment variable handling works for sericon tools
- Check that any new dependencies are properly declared in go.mod

## Repository Structure

### Key Components
- **Library Package**: `pkg/azconsoles/azconsoles.go` - Core Azure console connectivity
- **CLI Tools**:
  - `cmd/azconsoles/cloudshell.go` - Azure Cloud Shell client
  - `cmd/azconsoles/sericon.go` - Basic serial console client (read-only)
  - `cmd/azconsoles/sericon-pty.go` - Advanced serial console with PTY support
- **Scripts**: `scripts/ppp-client-setup.sh` - PPP network setup for serial console
- **Build**: `Makefile` - Multi-binary build configuration

### Dependencies
- Azure SDK for Go (azcore, azidentity, armserialconsole)
- WebSocket: github.com/gobwas/ws
- PTY: github.com/creack/pty
- UUID: github.com/google/uuid
- Terminal: golang.org/x/term

## Common Tasks and Timing

### Build Times (NEVER CANCEL)
- `go mod tidy`: 10-15 seconds (first time), <1 second (subsequent)
- `make all`: 1-25 seconds (1.5s subsequent, 21s first time), set timeout to 60+ seconds
- `go fmt ./...`: <1 second
- `go vet ./pkg/...`: <2 seconds
- `go test ./pkg/...`: <1 second

### Frequently Used Commands
```bash
# Complete development cycle
go mod tidy && go fmt ./... && go vet ./pkg/... && make clean && make all

# Check what will be formatted (should be empty after fixing)
go fmt ./...

# Build specific binary
go build -o bin/cloudshell ./cmd/azconsoles/cloudshell.go

# Test library package only (avoid cmd/ due to multiple main functions)
go test ./pkg/...
```

### Repository Root Contents
```
.
├── Makefile              # Build configuration
├── README.md             # Basic usage documentation
├── cmd/                  # CLI applications
│   └── azconsoles/
│       ├── cloudshell.go    # Azure Cloud Shell client
│       ├── sericon.go       # Basic serial console
│       └── sericon-pty.go   # PTY serial console
├── go.mod                # Go module definition
├── go.sum                # Dependency checksums
├── pkg/                  # Library packages
│   └── azconsoles/
│       └── azconsoles.go    # Core library
└── scripts/              # Utility scripts
    └── ppp-client-setup.sh  # PPP network setup
```

## Known Limitations and Workarounds

### Testing Limitations
- No unit tests exist in the repository
- CLI applications require Azure authentication, making automated testing complex
- Use `go test ./pkg/...` instead of `go test ./...` to avoid multiple main function conflicts

### Development Without Azure Access
- Build validation works without Azure credentials
- Applications will panic with authentication errors (expected behavior)
- Use build success and graceful failure as validation criteria

### Code Quality
- **ALWAYS** run `go fmt ./...` before committing
- No linting configuration exists - rely on `go vet` and `go fmt`
- No CI/CD workflows exist - manual validation is required

## Authentication Error Examples (Expected Behavior)
```
panic: DefaultAzureCredential: failed to acquire a token.
    Attempted credentials:
        EnvironmentCredential: missing environment variable AZURE_TENANT_ID
        WorkloadIdentityCredential: no client ID specified
        ManagedIdentityCredential: managed identity timed out
        AzureCLICredential: ERROR: Please run 'az login' to setup account
        AzureDeveloperCLICredential: Azure Developer CLI not found on path
```
This panic is expected when running applications without proper Azure authentication setup.

## Development Workflow
1. Make code changes
2. Run `go fmt ./...` to format code
3. Run `go vet ./pkg/...` to check for issues
4. Run `make clean && make all` to rebuild all binaries
5. Test that binaries execute (they may panic due to auth - this is expected)
6. Commit changes

Always follow this workflow to ensure code quality and build consistency.