# Pooshit 💩 (pronounced Push-It)

A Go application that pushes (and pulls) files between local and remote servers via SFTP, then manages Docker containers on the remote server through SSH.

**Why "Pooshit"?** Because you're literally pushing your... code... to remote servers without server-side or cloud services. That doesn't really explain the name but hopefully you get it. This is for the laziest of developers who want to get their local code running in remote containers with the least effort (I think).

## Features

- **Bidirectional Sync**: Push local files to remote or pull remote files to local
- **SFTP File Synchronization**: Automatically syncs your local development folder to a remote server
- **Docker Container Management**: Stops and removes existing containers, rebuilds images, and deploys new containers
- **Configuration-based**: Simple configuration file for managing multiple projects
- **Automatic Change Detection**: Only transfers files that have been modified
- **Automatic Directory Creation**: Creates remote/local directories if they don't exist
- **Ignore Patterns**: Exclude files and directories from sync (e.g., node_modules, .git)
- **Progress Bar**: Clean progress visualization during file synchronization
- **Smart Logging**: Concise output with emojis for better readability

## Installation

1. Make sure you have Go 1.21 or later installed
2. Clone this repository or copy the files to your project directory
3. Install dependencies:

```bash
go mod download
```

## Quick Start

1. Copy the example configuration:
   ```bash
   cp pooshit_config.example pooshit_config
   ```

2. Edit `pooshit_config` with your server details and project settings

3. Build and run:
   ```bash
   go build -o pooshit
   ./pooshit
   ```

## Configuration

Create a `pooshit_config` file in your project directory. You can copy from `pooshit_config.example` as a starting point:

```
REMOTE_SERVER: your.server.com
SSH_USERNAME: your_username
SSH_PASSWORD: your_password
REMOTE_FOLDER: ~/projects/project1
LOCAL_FOLDER: ./
DOCKER_IMAGE_NAME: your_image_name
DOCKER_BUILD_ARGS: -t
DOCKER_RUN_ARGS: --restart unless-stopped -p 8080:3000 -d -e VIRTUAL_HOST=hostname.remote.com
IGNORE: node_modules/, .git, *.env, *.log, dist/, build/
```

### Configuration Options

- **REMOTE_SERVER**: The hostname or IP address of your remote server (port 22 is used by default, or specify as `host:port`)
- **SSH_USERNAME**: SSH username for authentication
- **SSH_PASSWORD**: SSH password for authentication
- **REMOTE_FOLDER**: The destination folder on the remote server (supports `~` for home directory)
- **LOCAL_FOLDER**: The local folder to sync (defaults to current directory if not specified)
- **DOCKER_IMAGE_NAME**: Name of the Docker image to build and run
- **DOCKER_BUILD_ARGS**: Additional arguments for `docker build` command (defaults to `-t`)
- **DOCKER_RUN_ARGS**: Arguments for `docker run` command
- **IGNORE**: Comma-separated list of patterns to exclude from sync (optional)

### Ignore Patterns

The `IGNORE` option supports several pattern types:

- **Directory patterns**: Use directory name with or without trailing `/` (e.g., `node_modules` or `node_modules/`)
- **File patterns**: Use wildcards for file matching (e.g., `*.env`, `*.log`, `*.tmp`)
- **Exact matches**: Specify exact file or directory names (e.g., `.git`, `.DS_Store`)

Common ignore patterns:
```
IGNORE: node_modules, .git, *.env, *.log, dist, build, .DS_Store, *.swp, *.tmp
```

**Note**: The application automatically recognizes directory patterns and will skip the entire directory tree when matched.

If no `IGNORE` option is provided, these patterns are ignored by default:
- `.git`, `.gitignore`, `.env`, `*.swp`, `*.tmp`

## Usage

### Build the application:

#### Windows:
```bash
build.bat
# Or manually:
go build -o pooshit.exe
```

#### Linux/Mac:
```bash
chmod +x build.sh
./build.sh
# Or manually:
go build -o pooshit
```

### Push mode (default) - Upload local files to remote and manage Docker:

```bash
# With default config file (pooshit_config)
./pooshit

# With custom config file
./pooshit custom_config
```

### Pull mode - Download remote files to local:

```bash
# Pull with default config file
./pooshit pull

# Pull with custom config file
./pooshit custom_config pull
```

**Note**: Pull mode will ask for confirmation before overwriting local files. No Docker operations are performed in pull mode.

## Workflow

### Push Mode (Default)

The application performs the following steps:

1. **Connect**: Establishes SSH and SFTP connections to the remote server
2. **Create Remote Directory**: Automatically creates the remote folder if it doesn't exist
3. **Sync Files**: Uploads files from local to remote folder
   - Skips files and directories matching ignore patterns
   - Only uploads modified files
   - Shows progress bar with current operation
4. **Stop Containers**: Stops and removes any running Docker containers using the specified image
5. **Remove Image**: Removes the existing Docker image
6. **Build Image**: Builds a new Docker image from the Dockerfile in the remote folder
7. **Run Container**: Starts a new container with the specified run arguments

### Pull Mode

When run with the `pull` parameter:

1. **Connect**: Establishes SSH and SFTP connections to the remote server
2. **Confirm**: Asks for user confirmation before proceeding
3. **Create Local Directory**: Automatically creates the local folder if it doesn't exist
4. **Pull Files**: Downloads files from remote to local folder
   - Skips files and directories matching ignore patterns
   - Only downloads modified files
   - Shows progress bar with current operation
5. **Complete**: No Docker operations are performed

## Examples

### Common Use Cases

#### Deploy and run application:
```bash
./pooshit
```

#### Pull latest changes from production:
```bash
./pooshit pull
# Confirms: This will overwrite local files with remote files. Continue? (Y/n):
```

#### Use different configs for different environments:
```bash
# Development
./pooshit dev_config

# Production  
./pooshit prod_config

# Pull from staging
./pooshit staging_config pull
```

## Example Dockerfile

Make sure your project includes a `Dockerfile` in the root directory. Here's a simple example:

```dockerfile
FROM node:18-alpine
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
EXPOSE 3000
CMD ["npm", "start"]
```

## Security Considerations

- The current implementation uses password authentication and ignores host key verification for simplicity
- For production use, consider:
  - Using SSH key-based authentication instead of passwords
  - Implementing proper host key verification
  - Storing credentials securely (environment variables, encrypted config, etc.)
  - Using a secrets management system

## Enhanced Output

The application provides clean, organized output with:

- **Progress Bar**: Visual progress indicator during file synchronization
- **Smart Summaries**: Shows total files checked, uploaded, and skipped
- **Clean Docker Output**: Concise status updates with emojis for each Docker operation
- **Dockerfile Detection**: Warns you if no Dockerfile is found before starting
- **Error Context**: Only shows detailed output when errors occur
- **Real-time Build Output**: Shows Docker build progress as it happens

## Troubleshooting

### Connection Issues
- Verify the remote server address and port
- Check firewall settings on both local and remote machines
- Ensure SSH service is running on the remote server

### File Sync Issues
- Check the logs to see which files are being found locally
- Verify the `LOCAL_FOLDER` path in your config points to the correct directory
- Ensure the Dockerfile exists in your local folder (not in .gitignore)
- Check file permissions on the remote server
- Verify you have write permissions to the remote directory

### Docker Permission Issues
- The application now uses `sudo` for all Docker commands
- If you still get permission errors, ensure the SSH user is in the docker group:
  ```bash
  sudo usermod -aG docker $USER
  ```
- Or ensure the user has sudo privileges without password for Docker commands

### Docker Build Issues
- **"No such file or directory" for Dockerfile**: 
  - Check that the Dockerfile exists in your LOCAL_FOLDER
  - Ensure it's not being skipped (hidden files starting with . are skipped)
  - Verify the sync completed successfully by checking the logs
- Review Docker build and run arguments for correctness
- Check the remote directory listing in the logs to confirm files were synced

## Dependencies

- [github.com/pkg/sftp](https://github.com/pkg/sftp) - SFTP client library
- [golang.org/x/crypto/ssh](https://golang.org/x/crypto) - SSH client library
- [github.com/joho/godotenv](https://github.com/joho/godotenv) - Environment file support

## License

MIT License - feel free to use this code in your projects.

## Contributing

Contributions are welcome! Please feel free to submit pull requests or open issues for bugs and feature requests.
