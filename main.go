package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Config holds the application configuration
type Config struct {
	RemoteServer     string
	SSHUsername      string
	SSHPassword      string
	RemoteFolder     string
	LocalFolder      string
	DockerImageName  string
	DockerBuildArgs  string
	DockerRunArgs    string
	IgnorePatterns   []string
}

// SyncManager handles the synchronization and Docker operations
type SyncManager struct {
	config     *Config
	sshClient  *ssh.Client
	sftpClient *sftp.Client
}

// ProgressBar represents a simple progress bar
type ProgressBar struct {
	total   int
	current int
	width   int
	lastMsg string
}

// NewProgressBar creates a new progress bar
func NewProgressBar(total int) *ProgressBar {
	return &ProgressBar{
		total:   total,
		current: 0,
		width:   50,
	}
}

// Update updates the progress bar
func (p *ProgressBar) Update(current int, message string) {
	p.current = current
	p.lastMsg = message
	p.Draw()
}

// Draw draws the progress bar
func (p *ProgressBar) Draw() {
	if p.total == 0 {
		return
	}
	
	percent := float64(p.current) / float64(p.total)
	filledWidth := int(percent * float64(p.width))
	
	// Clear the line
	fmt.Print("\r\033[K")
	
	// Draw progress bar
	fmt.Print("[")
	for i := 0; i < p.width; i++ {
		if i < filledWidth {
			fmt.Print("=")
		} else if i == filledWidth {
			fmt.Print(">")
		} else {
			fmt.Print(" ")
		}
	}
	fmt.Printf("] %3d%% (%d/%d)\n", int(percent*100), p.current, p.total)
	
	// Show current operation on the next line
	if p.lastMsg != "" {
		fmt.Printf("\r\033[K%s", p.lastMsg)
	}
	
	// Move cursor up one line for next update
	if p.current < p.total {
		fmt.Print("\033[1A")
	}
}

// Complete marks the progress as complete
func (p *ProgressBar) Complete() {
	p.current = p.total
	p.Draw()
	fmt.Println() // Add extra newline after completion
}

// confirmAction prompts the user for a yes/no confirmation
func confirmAction(prompt string) bool {
	fmt.Printf("%s (Y/n): ", prompt)
	var response string
	fmt.Scanln(&response)
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "" || response == "y" || response == "yes"
}

// LoadConfig loads configuration from a file
func LoadConfig(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	config := &Config{}
	scanner := bufio.NewScanner(file)
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		
		switch key {
		case "REMOTE_SERVER":
			config.RemoteServer = value
		case "SSH_USERNAME":
			config.SSHUsername = value
		case "SSH_PASSWORD":
			config.SSHPassword = value
		case "REMOTE_FOLDER":
			config.RemoteFolder = value
		case "LOCAL_FOLDER":
			config.LocalFolder = value
		case "DOCKER_IMAGE_NAME":
			config.DockerImageName = value
		case "DOCKER_BUILD_ARGS":
			config.DockerBuildArgs = value
		case "DOCKER_RUN_ARGS":
			config.DockerRunArgs = value
		case "IGNORE":
			// Parse comma-separated ignore patterns
			patterns := strings.Split(value, ",")
			for _, pattern := range patterns {
				pattern = strings.TrimSpace(pattern)
				if pattern != "" {
					config.IgnorePatterns = append(config.IgnorePatterns, pattern)
				}
			}
		}
	}
	
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}
	
	// Validate required fields
	if config.RemoteServer == "" || config.SSHUsername == "" || config.SSHPassword == "" ||
		config.RemoteFolder == "" || config.DockerImageName == "" {
		return nil, fmt.Errorf("missing required configuration fields")
	}
	
	// Default local folder to current directory if not specified
	if config.LocalFolder == "" {
		config.LocalFolder = "."
	}
	
	// Add default ignore patterns if none specified
	if len(config.IgnorePatterns) == 0 {
		config.IgnorePatterns = []string{".git", ".gitignore", ".env", "*.swp", "*.tmp"}
	}
	
	return config, nil
}

// NewSyncManager creates a new sync manager instance
func NewSyncManager(config *Config) (*SyncManager, error) {
	return &SyncManager{
		config: config,
	}, nil
}

// Connect establishes SSH and SFTP connections
func (sm *SyncManager) Connect() error {
	// SSH configuration
	sshConfig := &ssh.ClientConfig{
		User: sm.config.SSHUsername,
		Auth: []ssh.AuthMethod{
			ssh.Password(sm.config.SSHPassword),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // In production, use proper host key verification
		Timeout:         10 * time.Second,
	}
	
	// Add port if not specified
	addr := sm.config.RemoteServer
	if !strings.Contains(addr, ":") {
		addr = addr + ":22"
	}
	
	// Connect via SSH
	sshClient, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return fmt.Errorf("failed to connect via SSH: %w", err)
	}
	sm.sshClient = sshClient
	
	// Create SFTP client
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		sm.sshClient.Close()
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	sm.sftpClient = sftpClient
	
	log.Printf("\n✅ Connected to %s", sm.config.RemoteServer)
	return nil
}

// Close closes all connections
func (sm *SyncManager) Close() {
	if sm.sftpClient != nil {
		sm.sftpClient.Close()
	}
	if sm.sshClient != nil {
		sm.sshClient.Close()
	}
}

// shouldIgnore checks if a file/directory should be ignored based on patterns
func (sm *SyncManager) shouldIgnore(relPath string, info os.FileInfo) bool {
	baseName := filepath.Base(relPath)
	relPathSlash := filepath.ToSlash(relPath)
	
	for _, pattern := range sm.config.IgnorePatterns {
		// Clean up pattern - remove leading slashes
		pattern = strings.TrimPrefix(pattern, "/")
		pattern = strings.TrimPrefix(pattern, "./")
		
		// Check if it's explicitly a directory pattern (ends with /)
		isDirectoryPattern := strings.HasSuffix(pattern, "/")
		if isDirectoryPattern {
			pattern = strings.TrimSuffix(pattern, "/")
		}
		
		// For directory patterns or patterns without wildcards, check directory names
		if isDirectoryPattern || !strings.Contains(pattern, "*") {
			// Check if this is the directory itself
			if info.IsDir() && (baseName == pattern || matchPattern(baseName, pattern)) {
				return true
			}
			
			// Check if any parent directory matches
			pathParts := strings.Split(relPathSlash, "/")
			for _, part := range pathParts {
				if part == pattern || matchPattern(part, pattern) {
					return true
				}
			}
		}
		
		// For file patterns (containing wildcards)
		if strings.Contains(pattern, "*") {
			if matchPattern(baseName, pattern) {
				return true
			}
		}
	}
	
	return false
}

// matchPattern checks if a string matches a simple glob pattern
func matchPattern(str, pattern string) bool {
	// Handle simple wildcard patterns
	if strings.Contains(pattern, "*") {
		// Use filepath.Match for glob pattern matching
		matched, _ := filepath.Match(pattern, str)
		return matched
	}
	// Exact match
	return str == pattern
}

// SyncFiles synchronizes local folder to remote folder
func (sm *SyncManager) SyncFiles() error {
	log.Printf("Starting file synchronization from '%s' to '%s'...", sm.config.LocalFolder, sm.config.RemoteFolder)
	
	if len(sm.config.IgnorePatterns) > 0 {
		log.Printf("Ignoring patterns: %s", strings.Join(sm.config.IgnorePatterns, ", "))
	}
	
	// Check if local folder exists
	localInfo, err := os.Stat(sm.config.LocalFolder)
	if err != nil {
		return fmt.Errorf("local folder '%s' does not exist or cannot be accessed: %w", sm.config.LocalFolder, err)
	}
	if !localInfo.IsDir() {
		return fmt.Errorf("local path '%s' is not a directory", sm.config.LocalFolder)
	}
	
	// Expand tilde in remote folder path
	remotePath := sm.config.RemoteFolder
	if strings.HasPrefix(remotePath, "~/") {
		homeDir, err := sm.getRemoteHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get remote home directory: %w", err)
		}
		remotePath = filepath.Join(homeDir, remotePath[2:])
	}
	log.Printf("Resolved remote path: %s", remotePath)
	
	// Create remote directory if it doesn't exist
	if err := sm.sftpClient.MkdirAll(remotePath); err != nil {
		log.Printf("Warning: Could not create remote directory (may already exist): %v", err)
	}
	
	// First pass: count total files to sync
	log.Print("Scanning local directory...")
	var filesToSync []struct {
		localPath  string
		remotePath string
		relPath    string
		info       os.FileInfo
	}
	ignored := 0
	
	err = filepath.Walk(sm.config.LocalFolder, func(localPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Get relative path
		relPath, err := filepath.Rel(sm.config.LocalFolder, localPath)
		if err != nil {
			return err
		}
		
		// Skip the root directory itself
		if relPath == "." {
			return nil
		}
		
		// Check if file/directory should be ignored
		if sm.shouldIgnore(relPath, info) {
			ignored++
			if info.IsDir() {
				// Log when skipping a directory for debugging
				if relPath == "node_modules" || strings.Contains(relPath, "node_modules") {
					log.Printf("Skipping directory: %s", relPath)
				}
				return filepath.SkipDir
			}
			return nil
		}
		
		if !info.IsDir() {
			remoteFilePath := filepath.Join(remotePath, relPath)
			remoteFilePath = filepath.ToSlash(remoteFilePath)
			
			filesToSync = append(filesToSync, struct {
				localPath  string
				remotePath string
				relPath    string
				info       os.FileInfo
			}{
				localPath:  localPath,
				remotePath: remoteFilePath,
				relPath:    relPath,
				info:       info,
			})
		} else {
			// Create directory on remote
			remoteFilePath := filepath.Join(remotePath, relPath)
			remoteFilePath = filepath.ToSlash(remoteFilePath)
			sm.sftpClient.MkdirAll(remoteFilePath)
		}
		
		return nil
	})
	
	if err != nil {
		return fmt.Errorf("failed to scan local directory: %w", err)
	}
	
	if len(filesToSync) == 0 {
		log.Println("No files to sync")
		if ignored > 0 {
			log.Printf("(%d files/directories ignored based on patterns)", ignored)
		}
		return nil
	}
	
	log.Printf("Found %d files to check (%d ignored)", len(filesToSync), ignored)
	
	// Create progress bar
	progressBar := NewProgressBar(len(filesToSync))
	
	// Second pass: sync files with progress bar
	skippedCount := 0
	syncedCount := 0
	
	for i, file := range filesToSync {
		// Check if file needs to be updated
		needsUpdate := true
		remoteInfo, err := sm.sftpClient.Stat(file.remotePath)
		if err == nil {
			// File exists, check if it needs updating (simple size and time comparison)
			if remoteInfo.Size() == file.info.Size() && remoteInfo.ModTime().After(file.info.ModTime().Add(-time.Second)) {
				needsUpdate = false
				skippedCount++
				progressBar.Update(i+1, fmt.Sprintf("Skipped (up-to-date): %s", file.relPath))
			}
		}
		
		if needsUpdate {
			progressBar.Update(i+1, fmt.Sprintf("Uploading: %s (%d bytes)", file.relPath, file.info.Size()))
			if err := sm.uploadFile(file.localPath, file.remotePath); err != nil {
				progressBar.Complete()
				return fmt.Errorf("failed to upload %s: %w", file.localPath, err)
			}
			syncedCount++
		} else {
			progressBar.Update(i+1, fmt.Sprintf("Checking: %s", file.relPath))
		}
	}
	
	progressBar.Complete()
	log.Printf("File synchronization completed: %d files checked, %d uploaded, %d already up-to-date", 
		len(filesToSync), syncedCount, skippedCount)
	if ignored > 0 {
		log.Printf("(%d files/directories ignored based on patterns)", ignored)
	}
	
	// Check if Dockerfile exists in the synced files
	dockerfilePath := filepath.Join(sm.config.LocalFolder, "Dockerfile")
	if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
		log.Printf("WARNING: No Dockerfile found in local folder '%s'", sm.config.LocalFolder)
	}
	
	return nil
}

// PullFiles downloads files from remote to local (reverse sync)
func (sm *SyncManager) PullFiles() error {
	log.Printf("Starting file pull from '%s' to '%s'...", sm.config.RemoteFolder, sm.config.LocalFolder)
	
	if len(sm.config.IgnorePatterns) > 0 {
		log.Printf("Ignoring patterns: %s", strings.Join(sm.config.IgnorePatterns, ", "))
	}
	
	// Expand tilde in remote folder path
	remotePath := sm.config.RemoteFolder
	if strings.HasPrefix(remotePath, "~/") {
		homeDir, err := sm.getRemoteHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get remote home directory: %w", err)
		}
		remotePath = filepath.Join(homeDir, remotePath[2:])
	}
	remotePath = filepath.ToSlash(remotePath)
	log.Printf("Resolved remote path: %s", remotePath)
	
	// Create local directory if it doesn't exist
	if err := os.MkdirAll(sm.config.LocalFolder, 0755); err != nil {
		return fmt.Errorf("failed to create local directory: %w", err)
	}
	
	// Walk through remote directory and pull files
	log.Print("Scanning remote directory...")
	var filesToPull []struct {
		localPath  string
		remotePath string
		relPath    string
		info       os.FileInfo
	}
	ignored := 0
	
	// Use SFTP Walker to traverse remote directory
	walker := sm.sftpClient.Walk(remotePath)
	for walker.Step() {
		if err := walker.Err(); err != nil {
			continue
		}
		
		stat := walker.Stat()
		remoteFilePath := walker.Path()
		
		// Get relative path from remote base
		relPath, err := filepath.Rel(remotePath, remoteFilePath)
		if err != nil {
			continue
		}
		relPath = filepath.ToSlash(relPath)
		
		// Skip the root directory itself
		if relPath == "." {
			continue
		}
		
		// Check if file/directory should be ignored
		if sm.shouldIgnore(relPath, stat) {
			ignored++
			continue
		}
		
		if !stat.IsDir() {
			localPath := filepath.Join(sm.config.LocalFolder, filepath.FromSlash(relPath))
			
			filesToPull = append(filesToPull, struct {
				localPath  string
				remotePath string
				relPath    string
				info       os.FileInfo
			}{
				localPath:  localPath,
				remotePath: remoteFilePath,
				relPath:    relPath,
				info:       stat,
			})
		} else {
			// Create directory on local
			localDirPath := filepath.Join(sm.config.LocalFolder, filepath.FromSlash(relPath))
			os.MkdirAll(localDirPath, 0755)
		}
	}
	
	if len(filesToPull) == 0 {
		log.Println("No files to pull")
		if ignored > 0 {
			log.Printf("(%d files/directories ignored based on patterns)", ignored)
		}
		return nil
	}
	
	log.Printf("Found %d files to download (%d ignored)", len(filesToPull), ignored)
	
	// Create progress bar
	progressBar := NewProgressBar(len(filesToPull))
	
	// Pull files with progress bar
	downloadedCount := 0
	skippedCount := 0
	
	for i, file := range filesToPull {
		// Check if file needs to be updated
		needsUpdate := true
		localInfo, err := os.Stat(file.localPath)
		if err == nil {
			// File exists, check if it needs updating (simple size comparison)
			if localInfo.Size() == file.info.Size() && localInfo.ModTime().After(file.info.ModTime().Add(-time.Second)) {
				needsUpdate = false
				skippedCount++
				progressBar.Update(i+1, fmt.Sprintf("Skipped (up-to-date): %s", file.relPath))
			}
		}
		
		if needsUpdate {
			progressBar.Update(i+1, fmt.Sprintf("Downloading: %s (%d bytes)", file.relPath, file.info.Size()))
			if err := sm.downloadFile(file.remotePath, file.localPath); err != nil {
				progressBar.Complete()
				return fmt.Errorf("failed to download %s: %w", file.remotePath, err)
			}
			downloadedCount++
		} else {
			progressBar.Update(i+1, fmt.Sprintf("Checking: %s", file.relPath))
		}
	}
	
	progressBar.Complete()
	log.Printf("File pull completed: %d files checked, %d downloaded, %d already up-to-date", 
		len(filesToPull), downloadedCount, skippedCount)
	if ignored > 0 {
		log.Printf("(%d files/directories ignored based on patterns)", ignored)
	}
	
	return nil
}

// downloadFile downloads a single file via SFTP
func (sm *SyncManager) downloadFile(remotePath, localPath string) error {
	// Create directory for the file if it doesn't exist
	dir := filepath.Dir(localPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	
	// Open remote file
	remoteFile, err := sm.sftpClient.Open(remotePath)
	if err != nil {
		return fmt.Errorf("failed to open remote file: %w", err)
	}
	defer remoteFile.Close()
	
	// Get remote file info
	info, err := remoteFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat remote file: %w", err)
	}
	
	// Create local file
	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer localFile.Close()
	
	// Copy file contents
	_, err = io.Copy(localFile, remoteFile)
	if err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}
	
	// Try to preserve file permissions
	if err := os.Chmod(localPath, info.Mode()); err != nil {
		// Silently ignore permission errors on Windows
	}
	
	return nil
}

// uploadFile uploads a single file via SFTP
func (sm *SyncManager) uploadFile(localPath, remotePath string) error {
	// Open local file
	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer localFile.Close()
	
	// Get file info for size
	info, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat local file: %w", err)
	}
	
	// Create remote file
	remoteFile, err := sm.sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create remote file: %w", err)
	}
	defer remoteFile.Close()
	
	// Copy file contents
	_, err = io.Copy(remoteFile, localFile)
	if err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}
	
	// Copy file permissions
	if err := remoteFile.Chmod(info.Mode()); err != nil {
		// Silently ignore permission errors
	}
	
	return nil
}

// getRemoteHomeDir gets the remote home directory
func (sm *SyncManager) getRemoteHomeDir() (string, error) {
	session, err := sm.sshClient.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	
	output, err := session.Output("echo $HOME")
	if err != nil {
		return "", err
	}
	
	return strings.TrimSpace(string(output)), nil
}

// ExecuteDockerCommands runs Docker management commands on the remote server
func (sm *SyncManager) ExecuteDockerCommands() error {
	log.Println("\nManaging Docker containers and images...")
	
	// Expand tilde in remote folder path for Docker context
	remotePath := sm.config.RemoteFolder
	if strings.HasPrefix(remotePath, "~/") {
		homeDir, err := sm.getRemoteHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get remote home directory: %w", err)
		}
		remotePath = filepath.Join(homeDir, remotePath[2:])
	}
	remotePath = filepath.ToSlash(remotePath)
	
	// Check if Dockerfile exists in remote directory
	checkCmd := fmt.Sprintf("test -f %s/Dockerfile && echo 'Dockerfile found' || echo 'Dockerfile NOT found'", remotePath)
	if output, err := sm.executeRemoteCommandWithOutput(checkCmd, false); err == nil {
		if strings.Contains(output, "NOT found") {
			log.Printf("⚠️  WARNING: Dockerfile not found in %s", remotePath)
		}
	}
	
	// Step 1: Stop and remove running containers using the image
	log.Printf("🐳 Stopping containers using image: %s", sm.config.DockerImageName)
	cmd := fmt.Sprintf("sudo docker ps -aq --filter ancestor=%s | xargs -r sudo docker stop | xargs -r sudo docker rm",
		sm.config.DockerImageName)
	sm.executeRemoteCommandQuiet(cmd)
	
	// Step 2: Remove the Docker image
	log.Printf("🗑️  Removing old image: %s", sm.config.DockerImageName)
	cmd = fmt.Sprintf("sudo docker rmi -f %s 2>/dev/null || true", sm.config.DockerImageName)
	sm.executeRemoteCommandQuiet(cmd)
	
	// Step 3: Build the new Docker image
	log.Printf("🔨 Building new image: %s", sm.config.DockerImageName)
	
	buildArgs := sm.config.DockerBuildArgs
	if buildArgs == "" {
		buildArgs = "-t"
	}
	cmd = fmt.Sprintf("cd %s && sudo docker build %s %s .", remotePath, buildArgs, sm.config.DockerImageName)
	if err := sm.executeRemoteCommandWithProgress(cmd); err != nil {
		return fmt.Errorf("failed to build Docker image: %w", err)
	}
	
	// Step 4: Run the new container
	log.Printf("▶️  Starting container: %s", sm.config.DockerImageName)
	runArgs := sm.config.DockerRunArgs
	if runArgs == "" {
		runArgs = "-d"
	}
	cmd = fmt.Sprintf("sudo docker run %s %s", runArgs, sm.config.DockerImageName)
	if output, err := sm.executeRemoteCommandWithOutput(cmd, true); err != nil {
		return fmt.Errorf("failed to run Docker container: %w", err)
	} else if output != "" {
		log.Printf("✅ Container started with ID: %s", strings.TrimSpace(output))
	}
	
	log.Println("\n✨ Docker operations completed successfully!")
	return nil
}

// executeRemoteCommand executes a command on the remote server via SSH
func (sm *SyncManager) executeRemoteCommand(command string) error {
	log.Printf("Executing: %s", command)
	
	session, err := sm.sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()
	
	// Capture output for logging
	output, err := session.CombinedOutput(command)
	if len(output) > 0 {
		log.Printf("Output:\n%s", string(output))
	}
	
	if err != nil {
		return fmt.Errorf("command failed: %w", err)
	}
	
	return nil
}

// executeRemoteCommandQuiet executes a command without logging output unless there's an error
func (sm *SyncManager) executeRemoteCommandQuiet(command string) error {
	session, err := sm.sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()
	
	output, err := session.CombinedOutput(command)
	if err != nil && len(output) > 0 {
		log.Printf("Error output: %s", string(output))
	}
	
	return err
}

// executeRemoteCommandWithOutput executes a command and returns the output
func (sm *SyncManager) executeRemoteCommandWithOutput(command string, showErrors bool) (string, error) {
	session, err := sm.sshClient.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()
	
	output, err := session.CombinedOutput(command)
	if err != nil && showErrors {
		log.Printf("Command error: %v", err)
		if len(output) > 0 {
			log.Printf("Error output: %s", string(output))
		}
	}
	
	return string(output), err
}

// executeRemoteCommandWithProgress executes a command and shows output in real-time
func (sm *SyncManager) executeRemoteCommandWithProgress(command string) error {
	session, err := sm.sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()
	
	// Pipe stdout and stderr to display in real-time
	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return err
	}
	
	if err := session.Start(command); err != nil {
		return err
	}
	
	// Read output in real-time
	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)
	
	return session.Wait()
}

func showHelp() {
	fmt.Println(`
Pooshit - Push/Pull files and manage Docker containers on remote servers

Usage:
  pooshit [config_file] [mode]
  pooshit [mode] [config_file]
  
Modes:
  (default)    Push local files to remote and manage Docker containers
  pull         Pull remote files to local (no Docker operations)

Arguments:
  config_file  Path to configuration file (default: pooshit_config)

Examples:
  pooshit                    # Push with default config
  pooshit pull                # Pull with default config
  pooshit my_config          # Push with custom config
  pooshit my_config pull     # Pull with custom config
  pooshit pull my_config     # Pull with custom config (order doesn't matter)

Options:
  -h, --help   Show this help message

Pull mode will ask for confirmation before overwriting local files.
`)
}

func main() {
	// Parse command line arguments
	configFile := "pooshit_config"
	pullMode := false
	
	// Check for help or pull mode
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "-h" || os.Args[i] == "--help" {
			showHelp()
			return
		}
		if os.Args[i] == "pull" {
			pullMode = true
		} else if !strings.HasPrefix(os.Args[i], "-") {
			// Assume it's a config file if it doesn't start with -
			configFile = os.Args[i]
		}
	}
	
	// Show a fun header
	if !pullMode {
		fmt.Println("\n💩 Pooshit v1.0 - Let's push some... code!")
		fmt.Println("─────────────────────────────────────────")
	}
	
	// Load configuration
	config, err := LoadConfig(configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	
	log.Println("\n📋 Configuration loaded:")
	log.Printf("   Server: %s", config.RemoteServer)
	log.Printf("   User: %s", config.SSHUsername)
	log.Printf("   Remote: %s", config.RemoteFolder)
	log.Printf("   Local: %s", config.LocalFolder)
	log.Printf("   Image: %s", config.DockerImageName)
	if len(config.IgnorePatterns) > 0 {
		log.Printf("   Ignore: %s", strings.Join(config.IgnorePatterns, ", "))
	}
	
	// List local directory contents
	log.Printf("\n📁 Checking local directory: %s", config.LocalFolder)
	files, err := os.ReadDir(config.LocalFolder)
	if err != nil {
		log.Fatalf("Failed to read local directory: %v", err)
	}
	
	dockerfileFound := false
	fileCount := 0
	for _, file := range files {
		if !strings.HasPrefix(file.Name(), ".") {
			fileCount++
			if file.Name() == "Dockerfile" {
				dockerfileFound = true
			}
		}
	}
	
	log.Printf("   Found %d files/directories (excluding hidden)", fileCount)
	
	if !dockerfileFound {
		log.Printf("\n⚠️  WARNING: No Dockerfile found in '%s'", config.LocalFolder)
		log.Printf("   Docker build will fail without a Dockerfile!")
	} else {
		log.Printf("   ✅ Dockerfile found")
	}
	
	// Create sync manager
	syncManager, err := NewSyncManager(config)
	if err != nil {
		log.Fatalf("Failed to create sync manager: %v", err)
	}
	
	// Connect to remote server
	if err := syncManager.Connect(); err != nil {
		log.Fatalf("Failed to connect to remote server: %v", err)
	}
	defer syncManager.Close()
	
	if pullMode {
		// Pull mode: download from remote to local
		log.Println("\n📥 Pull mode: Downloading files from remote to local")
		
		// Ask for confirmation
		if !confirmAction("This will overwrite local files with remote files. Continue?") {
			log.Println("Pull operation cancelled")
			return
		}
		
		if err := syncManager.PullFiles(); err != nil {
			log.Fatalf("File pull failed: %v", err)
		}
		log.Println("\n✅ Pull completed successfully!")
	} else {
		// Normal mode: push to remote and manage Docker
		// Synchronize files
		if err := syncManager.SyncFiles(); err != nil {
			log.Fatalf("File synchronization failed: %v", err)
		}
		
		// Execute Docker commands
		if err := syncManager.ExecuteDockerCommands(); err != nil {
			log.Fatalf("Docker operations failed: %v", err)
		}
		
		log.Println("\n🎉 All operations completed successfully!")
	}
}
