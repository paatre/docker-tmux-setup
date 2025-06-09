# Docker Tmux Setup

This program (`dts`) automates the setup of a local Docker-based development environment within a tmux session. It splits the tmux window into multiple panes, each configured for a specific development task, such as running services, frontend builds, and code editing.

## Requirements

To build and run `dts`, you will need:
- Go (version 1.18 or later recommended)
- Docker
- Docker Compose
- tmux

## Build Instructions

To build the `dts` executable:
1. Run the Go build command (ensure you are in the project root directory):
   ```bash
   go build -o dts .
   ```
   This will create an executable file named `dts` in the current directory.

## Usage Instructions

To use `dts`, run the compiled binary from within an active tmux session:

```bash
./dts [OPTIONS]
```

**Important:**
- You **must** be inside an existing tmux session to run this program.
- A `docker-compose.yml` file is expected to be present in the directory where you execute `dts`.

### Options:

The following command-line options are available:

-   `-h, --help`: Display the help message and exit.
-   `-b, --build`: Rebuild the Docker images (equivalent to `docker compose up --build`). Defaults to `false`.
-   `-e <editor_command>, --editor <editor_command>`: Specify the command-line editor to use (e.g., `vim`, `nvim`, `emacs`). Defaults to the value of your `$EDITOR` environment variable.
-   `-i <infra_version>, --infra <infra_version>`: Specify the Haltu infra version (e.g., `lilium`, `tulip`). Defaults to `lilium`. This is used for specific image tags or local settings configurations.
-   `-s <service_name>, --service <service_name>`: Specify the main Docker Compose service name to manage and connect to. Defaults to `runserver`.
-   `-v, --vite`: Include an additional tmux pane for a Vite development server (assumes your service has an `npm start` script or similar for Vite). Defaults to `false`.

### Example:

```bash
# Use default settings (service "runserver", no Vite, editor from $EDITOR)
./dts

# Specify a service and include the Vite pane
./dts --service myservice --vite

# Use a specific editor and rebuild Docker images
./dts --editor nvim --build
```
