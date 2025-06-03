# 🐳 Docker Setup Guide for JSDif

This guide explains how to run and configure JSDif using Docker.

## 🚀 Quick Start

```bash
# Clone the repository
git clone https://github.com/behf/jsdif.git
cd jsdif

# Start the container
docker compose up -d
```

## 🔐 Authentication Setup

### Method 1: Using docker-compose.yml

1. Open `docker-compose.yml`
2. Uncomment and modify the command section:

```yaml
services:
  jsdif:
    # ... other configurations ...
    command: ["-u", "your_username", "--password", "your_password", "run"]
```

3. Restart the container:
```bash
docker compose down
docker compose up -d
```

### Method 2: Using Environment Variables

1. Create a `.env` file:
```bash
JSDIF_USERNAME=your_username
JSDIF_PASSWORD=your_password
```

2. Modify `docker-compose.yml`:
```yaml
services:
  jsdif:
    # ... other configurations ...
    environment:
      - TZ=UTC
      - JSDIF_USERNAME=${JSDIF_USERNAME}
      - JSDIF_PASSWORD=${JSDIF_PASSWORD}
    command: ["-u", "${JSDIF_USERNAME}", "--password", "${JSDIF_PASSWORD}", "run"]
```

## 🔌 Port Configuration

### Default Port (9093)

The default configuration in `docker-compose.yml` uses port 9093:
```yaml
ports:
  - "9093:9093"
```

### Custom Port

To use a different port (e.g., 8080), modify the ports section:
```yaml
ports:
  - "8080:9093"  # Host port 8080 maps to container port 9093
```

## 💾 Data Persistence

JSDif stores JavaScript snapshots in a Docker volume. This is automatically configured in `docker-compose.yml`:

```yaml
volumes:
  - js_snapshots:/app/js_snapshots
```

To manage the volume:
```bash
# List volumes
docker volume ls

# Backup volume
docker run --rm -v jsdif_js_snapshots:/source -v $(pwd):/backup alpine tar czf /backup/js_snapshots_backup.tar.gz -C /source .

# Restore volume
docker run --rm -v jsdif_js_snapshots:/target -v $(pwd):/backup alpine sh -c "cd /target && tar xzf /backup/js_snapshots_backup.tar.gz"
```

## 🔄 Container Management

### Start Container
```bash
docker compose up -d
```

### Stop Container
```bash
docker compose down
```

### View Logs
```bash
docker compose logs -f
```

### Restart Container
```bash
docker compose restart
```

### Rebuild Container
```bash
docker compose build --no-cache
docker compose up -d
```

## 🔍 Health Check

Check if the container is running:
```bash
docker compose ps
```

Test the web interface:
```bash
curl http://localhost:9093  # Replace port if customized
```

## 🌍 Timezone Configuration

The container uses UTC by default. To use a different timezone:

1. Modify the environment section in `docker-compose.yml`:
```yaml
environment:
  - TZ=Your/Timezone  # e.g., Europe/Istanbul
```

2. Restart the container:
```bash
docker compose restart
```

## ⚠️ Troubleshooting

1. **Container won't start**
   - Check logs: `docker compose logs`
   - Verify port availability: `lsof -i :9093`
   - Ensure volume permissions are correct

2. **Web interface not accessible**
   - Verify container is running: `docker compose ps`
   - Check port mapping: `docker compose port jsdif 9093`
   - Ensure firewall allows the port

3. **Authentication issues**
   - Double-check username/password in configuration
   - Verify environment variables are passed correctly
   - Check for special characters in passwords

4. **Volume issues**
   - List volumes: `docker volume ls`
   - Inspect volume: `docker volume inspect jsdif_js_snapshots`
   - Check permissions: `docker exec -it jsdif ls -la /app/js_snapshots`

## 🔒 Security Best Practices

1. Always use authentication in production
2. Use strong passwords
3. Consider using a reverse proxy (like Nginx) for SSL termination
4. Regularly backup the js_snapshots volume
5. Keep the Docker image updated
6. Use specific versions in Dockerfile instead of 'latest' tag
