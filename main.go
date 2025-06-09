package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	buildFlag   bool
	editorFlag  string
	helpFlag    bool
	infraFlag   string
	serviceFlag string
	viteFlag    bool
)

// ComposeService represents a single service in docker-compose
type ComposeService struct {
	Ports  []string `yaml:"ports"`
	Expose []string `yaml:"expose"`
}

// ComposeConfig represents the docker-compose.yml structure
type ComposeConfig struct {
	Services map[string]ComposeService `yaml:"services"`
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "Options:")
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\nExample: %s --build -e nvim --service api\n", os.Args[0])
}

func validateEditor(editorCmd string) {
	if editorCmd == "" {
		return
	}
	_, err := exec.LookPath(editorCmd)
	if err != nil {
		log.Fatalf("Error: Specified editor '%s' is not installed or not in PATH.", editorCmd)
	}
}

func getContainerHealthPort(serviceName string, composeFilePath string) string {
	yamlFile, err := os.ReadFile(composeFilePath)
	if err != nil {
		log.Printf("Warning: Could not read docker-compose file at '%s': %v. Defaulting to port 8000.", composeFilePath, err)
		return "8000"
	}

	var config ComposeConfig
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		log.Printf("Warning: Could not unmarshal YAML from '%s': %v. Defaulting to port 8000.", composeFilePath, err)
		return "8000"
	}

	service, ok := config.Services[serviceName]
	if !ok {
		log.Printf("Warning: Service '%s' not found in %s. Defaulting to port 8000.", serviceName, composeFilePath)
		return "8000"
	}

	if len(service.Ports) > 0 {
		for _, portEntry := range service.Ports {
			parts := strings.Split(portEntry, ":")
			var portStr string
			if len(parts) > 1 {
				portStr = parts[len(parts)-1]
			} else {
				portStr = parts[0]
			}
			if _, err := strconv.Atoi(portStr); err == nil {
				log.Printf("Info: Found container port %s for service '%s' in 'ports' configuration.", portStr, serviceName)
				return portStr
			}
			log.Printf("Warning: Invalid port format '%s' for service '%s' in 'ports'. Skipping.", portEntry, serviceName)
		}
	}

	if len(service.Expose) > 0 {
		for _, portStr := range service.Expose {
			if _, err := strconv.Atoi(portStr); err == nil {
				log.Printf("Info: Found container port %s for service '%s' in 'expose' configuration.", portStr, serviceName)
				return portStr
			}
			log.Printf("Warning: Invalid port format '%s' for service '%s' in 'expose'. Skipping.", portStr, serviceName)
		}
	}

	switch serviceName {
	case "db", "postgres", "postgresql":
		log.Printf("Info: No explicit port found for DB service '%s'. Defaulting to 5432.", serviceName)
		return "5432"
	default:
		log.Printf("Warning: No usable port found for service '%s' in 'ports' or 'expose'. Defaulting to 8000.", serviceName)
		return "8000"
	}
}

func runCommand(command string, args ...string) (string, error) {
	cmd := exec.Command(command, args...)
	output, err := cmd.CombinedOutput()
	trimmedOutput := strings.TrimSpace(string(output))
	return trimmedOutput, err
}

func execTmuxCommand(args ...string) {
	fullCmd := "tmux " + strings.Join(args, " ")
	log.Printf("Executing tmux command: %s", fullCmd)
	output, err := runCommand("tmux", args...)
	if err != nil {
		log.Fatalf("tmux command failed: %s. Error: %v. Output: %s", fullCmd, err, output)
	}
	// log.Printf("tmux command successful: %s. Output: %s", fullCmd, output)
}

func waitForContainer(serviceName string, containerInternalPort string) {
	log.Printf("Attempting to find host port for service '%s' (internal port %s)...", serviceName, containerInternalPort)
	var hostPort string
	for {
		output, err := runCommand("docker", "compose", "port", serviceName, containerInternalPort)
		if err != nil || output == "" {
			time.Sleep(1 * time.Second)
			continue
		}
		parts := strings.Split(output, ":")
		if len(parts) < 2 {
			log.Printf("Error parsing host port output '%s' for service %s. Retrying...", output, serviceName)
			time.Sleep(1 * time.Second)
			continue
		}
		hostPort = parts[len(parts)-1]
		if _, err := strconv.Atoi(hostPort); err != nil {
			log.Printf("Error: Parsed host port '%s' for service %s is not a valid number. Output was '%s'. Retrying...", hostPort, serviceName, output)
			time.Sleep(1 * time.Second)
			continue
		}
		break
	}

	log.Printf("Service '%s' (internal port %s) is mapped to host port %s. Waiting for it to be ready...", serviceName, containerInternalPort, hostPort)

	switch serviceName {
	case "db", "postgres", "postgresql":
		log.Printf("Performing PostgreSQL health check for '%s' on internal port %s...", serviceName, containerInternalPort)
		for {
			_, err := runCommand("docker", "compose", "exec", "-T", serviceName, "pg_isready", "-h", "localhost", "-p", containerInternalPort, "-U", "postgres")
			if err == nil {
				break
			}
			log.Printf("PostgreSQL service '%s' not ready yet (internal port %s)... Error: %v", serviceName, containerInternalPort, err)
			time.Sleep(1 * time.Second)
		}
	default:
		log.Printf("Performing HTTP health check for '%s' on internal port %s...", serviceName, containerInternalPort)
		for {
			_, err := runCommand("docker", "compose", "exec", "-T", serviceName, "curl", "-sf", fmt.Sprintf("http://localhost:%s/", containerInternalPort))
			if err == nil {
				break
			}
			log.Printf("HTTP service '%s' not ready yet (internal port %s)... Error: %v", serviceName, containerInternalPort, err)
			time.Sleep(1 * time.Second)
		}
	}
	log.Printf("Service '%s' on host port %s (internal port %s) is ready!", serviceName, hostPort, containerInternalPort)
}

func main() {
	editorEnv := os.Getenv("EDITOR")
	editorHelpDefault := editorEnv
	if editorEnv == "" {
		editorHelpDefault = "not set (e.g., vim, nano, code)"
	}

	flag.BoolVar(&buildFlag, "build", false, "Rebuild the Docker images")
	flag.StringVar(&editorFlag, "editor", editorEnv, fmt.Sprintf("Specify the editor to use (default: %s)", editorHelpDefault))
	flag.BoolVar(&helpFlag, "help", false, "Display this help message")
	flag.StringVar(&infraFlag, "infra", "lilium", "Specify Haltu infra version (default: lilium)")
	flag.StringVar(&serviceFlag, "service", "runserver", "Specify the Docker service name (default: runserver)")
	flag.BoolVar(&viteFlag, "vite", false, "Include Vite development server pane")

	flag.BoolVar(&buildFlag, "b", false, "Rebuild the Docker images (shorthand for --build)")
	flag.StringVar(&editorFlag, "e", editorEnv, fmt.Sprintf("Specify the editor to use (shorthand for --editor; default: %s)", editorHelpDefault))
	flag.BoolVar(&helpFlag, "h", false, "Display this help message (shorthand for --help)")
	flag.StringVar(&infraFlag, "i", "lilium", "Specify Haltu infra version (shorthand for --infra; default: lilium)")
	flag.StringVar(&serviceFlag, "s", "runserver", "Specify the Docker service name (shorthand for --service; default: runserver)")
	flag.BoolVar(&viteFlag, "v", false, "Include Vite development server pane (shorthand for --vite)")

	flag.Usage = usage
	flag.Parse()

	if helpFlag {
		flag.Usage()
		os.Exit(0)
	}

	if len(flag.Args()) > 0 {
		fmt.Fprintf(os.Stderr, "Error: unknown arguments: %v\n", flag.Args())
		flag.Usage()
		os.Exit(1)
	}

	validateEditor(editorFlag)

	composeFilePath := "docker-compose.yml"
	containerHealthPort := getContainerHealthPort(serviceFlag, composeFilePath)

	if os.Getenv("TMUX") == "" {
		log.Fatal("This script must be run inside a tmux session.")
	}

	// This first split creates the main right pane (where Docker commands run) and the left pane (for the editor)
	// Let's assume the script itself is run in what will become the left pane, or tmux new-session.
	// The original script does `tmux new-session -d -s "$session_name"`. Then `tmux split-window -h`.
	// We are already inside a session. The first `split-window -h` effectively creates Pane 0 (left) and Pane 1 (right).
	// Pane 1 (right) becomes the target for subsequent splits and docker commands.
	log.Println("Splitting main window horizontally (for editor and services)...")
	execTmuxCommand("split-window", "-h")
	// At this point, the right pane is active. Let's call its ID (implicitly) "main_right_pane_target_for_docker_up"
	// However, tmux commands like send-keys need an explicit target.
	// The `docker compose up` command does not need to be sent to a specific pane via send-keys if we run it directly
	// and then manage other panes. The original script sends it to pane 1.
	// For simplicity, we'll assume the `docker compose up` runs in the context of the Go program,
	// and the tmux commands are for organizing other tools around it.
	// The `execTmuxCommand("split-window", "-h")` creates a new pane to the right of the current one.
	// The new (right) pane is active. This will be the "services" side.
	// The original (left) pane will be the "editor" side.

	// Docker operations
	if buildFlag {
		log.Println("Build flag is set. Performing Docker pull and local settings setup...")
		pullCommandArgs := []string{"pull", fmt.Sprintf("docker.haltu.net/haltu/env/%s:develop", infraFlag)}
		log.Printf("Executing Docker pull: docker %s", strings.Join(pullCommandArgs, " "))
		output, err := runCommand("docker", pullCommandArgs...)
		if err != nil {
			log.Printf("Error during Docker pull: %v\nOutput: %s", err, output)
		} else {
			log.Printf("Docker pull successful. Output: %s", output)
		}
		localSettingsCmd := fmt.Sprintf("cp -n ../local_settings.py local_settings_%s.py; cp -n local_settings.py.tpl_dev local_settings.py", infraFlag)
		localSettingsCommandArgs := []string{"compose", "run", "--rm", serviceFlag, "bash", "-c", localSettingsCmd}
		log.Printf("Executing local settings command: docker %s", strings.Join(localSettingsCommandArgs, " "))
		output, err = runCommand("docker", localSettingsCommandArgs...)
		if err != nil {
			log.Printf("Error during local settings command: %v\nOutput: %s", err, output)
		} else {
			log.Printf("Local settings command successful. Output: %s", output)
		}
	}

	log.Println("Starting Docker Compose up...")
	upCommandArgs := []string{"compose", "up"}
	if buildFlag {
		upCommandArgs = append(upCommandArgs, "--build")
	}
	upCommandArgs = append(upCommandArgs, "--remove-orphans")
	upCommandArgs = append(upCommandArgs, "-d")
	log.Printf("Executing Docker Compose up: docker %s", strings.Join(upCommandArgs, " "))
	output, err := runCommand("docker", upCommandArgs...)
	if err != nil {
		log.Fatalf("Failed to start Docker Compose services: %v\nOutput: %s", err, output)
	}
	log.Printf("Docker Compose up initiated successfully. Output: %s", output)

	log.Println("INFO: Calling waitForContainer after 'docker compose up -d'.")
	waitForContainer(serviceFlag, containerHealthPort)

	// TMUX Panes for Vite and Shell (on the right side)
	// The right side (currently active or selectable) is where these will go.
	// Let's assume the pane where 'docker compose up' logs would go (if not detached) or simply the main services activity pane
	// is the one we are splitting. We need its ID or select it first.
	// The first `split-window -h` made the right pane active. We'll call this the "main services pane".
	// Let's try to get the current pane ID for clarity, though tmux often acts on the current pane.
	// mainServicesPaneID, _ := runCommand("tmux", "display-message", "-p", "#{pane_id}")
	// log.Printf("Main services pane ID: %s", mainServicesPaneID) // For debugging

	var vitePaneID string
	if viteFlag {
		log.Println("Setting up Vite pane...")
		// This splits the current (main services) pane vertically. Vite is below.
		vitePaneOutputAndErr, err := runCommand("tmux", "split-window", "-v", "-P", "-F", "#{pane_id}")
		if err != nil || vitePaneOutputAndErr == "" {
			log.Fatalf("Failed to create Vite pane: %v. Output: %s", err, vitePaneOutputAndErr)
		}
		vitePaneID = strings.TrimSpace(vitePaneOutputAndErr)
		log.Printf("Vite pane created with ID: %s", vitePaneID)
		execTmuxCommand("send-keys", "-t", vitePaneID, "docker compose exec "+serviceFlag+" bash", "C-m")
		execTmuxCommand("send-keys", "-t", vitePaneID, "npm start", "C-m")
	}

	log.Println("Setting up interactive shell pane...")
	// This splits the currently active pane (Vite, or if no Vite, the main services pane) vertically. Shell is below.
	shellPaneOutputAndErr, err := runCommand("tmux", "split-window", "-v", "-P", "-F", "#{pane_id}")
	if err != nil || shellPaneOutputAndErr == "" {
		log.Fatalf("Failed to create shell pane: %v. Output: %s", err, shellPaneOutputAndErr)
	}
	shellPaneID := strings.TrimSpace(shellPaneOutputAndErr)
	log.Printf("Shell pane created with ID: %s", shellPaneID)
	execTmuxCommand("send-keys", "-t", shellPaneID, "docker compose exec "+serviceFlag+" bash", "C-m")
	execTmuxCommand("send-keys", "-t", shellPaneID, "source ../.venv/bin/activate", "C-m")
	execTmuxCommand("send-keys", "-t", shellPaneID, "clear", "C-m")

	log.Println("Resizing panes...")
	windowHeightOutput, err := runCommand("tmux", "display-message", "-p", "#{window_height}") // display-message to avoid issues with -p only
	if err != nil {
		log.Fatalf("Failed to get window height: %v. Output: %s", err, windowHeightOutput)
	}
	windowHeight, err := strconv.Atoi(strings.TrimSpace(windowHeightOutput))
	if err != nil {
		log.Fatalf("Failed to parse window height '%s': %v", windowHeightOutput, err)
	}

	// The pane that was active before the first split-window -v is the "main services activity" pane.
	// If viteFlag is true, order top-to-bottom: MainServices, Vite, Shell. All are children of the original right horizontal pane.
	// If viteFlag is false, order top-to-bottom: MainServices, Shell.
	// We need to target these panes for resizing. The IDs vitePaneID and shellPaneID are known.
	// The MainServices pane is the parent of vitePaneID (if vite) or shellPaneID (if not vite).
	// This can be found with `tmux list-panes -F "#{pane_id} #{pane_up}"`.
	// For simplicity, assume the current layout is a single column of 2 or 3 panes on the right.
	// The `split-window -v` command splits the *current* pane.
	// 1. Initial right pane (after -h split). Let's call it P_orig_right.
	// 2. If vite: P_orig_right splits, P_vite is new (bottom), P_orig_right is now P_top. P_vite active.
	// 3. P_vite splits, P_shell is new (bottom), P_vite is now P_middle. P_shell active.
	//    Order: P_top, P_middle (vitePaneID), P_shell (shellPaneID)
	// 4. If not vite: P_orig_right splits, P_shell is new (bottom), P_orig_right is now P_top. P_shell active.
	//    Order: P_top, P_shell (shellPaneID)

	if viteFlag { // 3 panes in the column: P_top, P_middle (vitePaneID), P_shell (shellPaneID)
		newHeight := windowHeight / 3
		execTmuxCommand("resize-pane", "-t", shellPaneID, "-y", strconv.Itoa(newHeight))    // Resize P_shell (bottom)
		execTmuxCommand("select-pane", "-t", vitePaneID)                                  // Select P_middle (Vite)
		execTmuxCommand("resize-pane", "-y", strconv.Itoa(newHeight))                     // Resize P_middle (Vite)
		execTmuxCommand("select-pane", "-U")                                              // Select P_top (relative to Vite)
		execTmuxCommand("resize-pane", "-y", strconv.Itoa(windowHeight-2*newHeight))      // Resize P_top (remaining)
	} else { // 2 panes in the column: P_top, P_shell (shellPaneID)
		newHeight := windowHeight / 2
		execTmuxCommand("resize-pane", "-t", shellPaneID, "-y", strconv.Itoa(newHeight)) // Resize P_shell (bottom)
		execTmuxCommand("select-pane", "-t", shellPaneID)                               // Select P_shell
		execTmuxCommand("select-pane", "-U")                                             // Select P_top (relative to Shell)
		execTmuxCommand("resize-pane", "-y", strconv.Itoa(windowHeight-newHeight))     // Resize P_top (remaining)
	}

	// Select the shell pane to leave it active for the user
	execTmuxCommand("select-pane", "-t", shellPaneID)


	log.Println("Setting up editor pane...")
	execTmuxCommand("select-pane", "-L") // Select the leftmost pane (Pane 0)
	if editorFlag != "" {
		execTmuxCommand("send-keys", "-t", "0", editorFlag+" .", "C-m") // Target pane 0 explicitly
	} else {
		log.Println("No editor specified, skipping editor pane setup.")
	}

	log.Println("Docker-tmux setup completed.")
	// Final focus on the interactive shell pane on the right.
	execTmuxCommand("select-pane", "-t", shellPaneID)


	fmt.Println("\nFinal state (after docker compose up and wait):")
	fmt.Printf("  build (-b, --build): %t\n", buildFlag)
	fmt.Printf("  editor (-e, --editor): %s\n", editorFlag)
	fmt.Printf("  infra (-i, --infra): %s\n", infraFlag)
	fmt.Printf("  service (-s, --service): %s\n", serviceFlag)
	fmt.Printf("  vite (-v, --vite): %t\n", viteFlag)
	fmt.Printf("  Container Health Port: %s\n", containerHealthPort)
}
