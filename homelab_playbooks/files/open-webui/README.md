# Open WebUI Chat Interface Deployment

This deployment provides a modern web chat interface for accessing the llama.cpp API server with GPU-accelerated inference.

## Features

- **Modern Chat UI**: Clean, responsive web interface for AI conversations
- **API Integration**: Seamlessly connects to local llama.cpp API server
- **User Management**: Built-in authentication and user accounts
- **Model Management**: Easy model selection and configuration
- **SQLite Database**: Uses local SQLite database for persistent storage
- **Security**: Sandboxed execution with restricted permissions
- **Systemd Integration**: Auto-starting service with health monitoring

## Architecture

```
Browser → nginx (port 80/443) → Open WebUI (port 3000) → Ollama API (port 11434) → GPU-accelerated inference
```

## Directory Structure

```
/data01/services/open-webui/
├── docker-compose.yml    # Generated from template
├── .env                 # Environment variables
├── data/                # User data and database
│   ├── webui.db        # SQLite database
│   └── uploads/        # File uploads
├── config/              # Configuration files
└── logs/                # Application logs
```

## Deployment

### Prerequisites

1. **Docker**: Ensure Docker and docker-compose are installed
2. **Storage**: Sufficient space in `/data01` for database and user data

### Configuration

The playbook uses SQLite for local data storage:

- **Database**: SQLite database stored in `/data01/services/open-webui/data/webui.db`
- **API Integration**: Pre-configured to connect to Ollama at port 11434
- **Auto-setup**: Creates all necessary directories and permissions automatically

### Deploy

Run the playbook to deploy:

```bash
ansible-playbook -i inventory.ini playbook_ocean_open_webui.yaml
```

The playbook will automatically:

- Create all necessary directories with correct permissions
- Configure Open WebUI to connect to your Ollama API
- Deploy the containerized service with SQLite storage

## Service Management

```bash
# Check service status
sudo systemctl status open-webui

# Start/stop service
sudo systemctl start open-webui
sudo systemctl stop open-webui

# View logs
sudo journalctl -u open-webui -f

# Check container logs
docker logs open-webui -f
```

## Access

### Web Interface
- **Local**: http://ocean:3000
- **External**: https://webui.terrac.com (if nginx/cloudflare configured)

### First Time Setup
1. Navigate to the web interface
2. Click "Sign Up" to create the first admin account
3. The first user becomes the administrator
4. Additional users can be created or signup can be disabled

## Configuration

### Model Configuration
The interface automatically detects the Phi-3.5-mini model running on llama.cpp.

### User Management
```bash
# Disable new user signups (edit environment file)
cd /data01/services/open-webui
sudo -u media nano .env
# Change: ENABLE_SIGNUP=false

# Restart service to apply changes
sudo systemctl restart open-webui
```

### API Settings
The service is pre-configured to connect to llama.cpp at `http://localhost:8080/v1`.

To use different models or endpoints, edit the environment file:
```bash
cd /data01/services/open-webui
sudo -u media nano .env
# Modify OPENAI_API_BASE_URL or DEFAULT_MODELS
sudo systemctl restart open-webui
```

## Usage Examples

### Basic Chat
1. Open http://ocean:3000
2. Sign in or create account
3. Select "Phi-3.5-mini-instruct-Q4_K_M.gguf" from model dropdown
4. Start chatting!

### Advanced Features
- **Temperature Control**: Adjust response creativity in chat settings
- **Context Management**: Configure conversation memory length
- **Chat History**: Previous conversations are automatically saved
- **Export Chats**: Download conversations as JSON or markdown

## Troubleshooting

### Common Issues

1. **Cannot connect to llama.cpp API**
   ```bash
   # Check if llama.cpp service is running
   sudo systemctl status llamacpp
   curl http://localhost:8080/v1/models
   ```

2. **Web interface not accessible**
   ```bash
   # Check Open WebUI container status
   docker ps | grep open-webui
   docker logs open-webui
   ```

3. **Database issues**
   ```bash
   # Reset database (WARNING: loses chat history)
   sudo systemctl stop open-webui
   sudo -u media rm /data01/services/open-webui/data/webui.db
   sudo systemctl start open-webui
   ```

### Health Checks
```bash
# Test web interface
curl -f http://localhost:3000/

# Test API connectivity through Open WebUI
curl -X GET http://localhost:3000/api/v1/models \
  -H "Authorization: Bearer your-session-token"
```

## Security Considerations

- **Authentication Required**: All users must sign in
- **No External Access**: By default, only accessible from local network
- **Sandboxed Container**: Runs with restricted permissions
- **Data Isolation**: User data stored in dedicated directory

## Integration with Other Services

### Nginx Reverse Proxy
Add to nginx configuration:
```nginx
location /ai/ {
    proxy_pass http://localhost:3000/;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    
    # WebSocket support
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
}
```

### Cloudflare Tunnel
The service can be exposed via Cloudflare tunnel for external access.

## Performance

- **Resource Usage**: ~200MB RAM, minimal CPU when idle
- **Response Times**: Depends on llama.cpp inference speed (~17 tokens/second)
- **Concurrent Users**: Supports multiple simultaneous chat sessions
- **Scalability**: Single llama.cpp instance handles multiple UI users

## Backup and Maintenance

### Backup User Data
```bash
# Backup database and user files
sudo tar -czf open-webui-backup-$(date +%Y%m%d).tar.gz \
  -C /data01/services open-webui/data
```

### Update Open WebUI
```bash
# Update to latest version
cd /data01/services/open-webui
sudo systemctl stop open-webui
sudo -u media docker-compose pull
sudo systemctl start open-webui
```

## Dependencies

- **llama.cpp service**: Must be running for AI functionality
- **Docker and docker-compose**: Container runtime
- **Network connectivity**: Internal network access to llama.cpp API
- **Storage**: Sufficient space in `/data01` for user data and chat history
