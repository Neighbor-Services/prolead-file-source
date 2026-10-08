#!/bin/bash
# ==============================================================================
# GoStore / Prolead File - Production Backend & Web Appliance Deployment Script
# Multi-Instance Go Cluster & Nginx Upstream Load Balancing (No Docker required)
# Domain: https://file.proleadsolutions.co
# Usage: sudo bash deploy-backend.sh
# ==============================================================================

set -e

# Trap errors and print the line number for easier debugging
trap 'echo -e "\033[0;31m[ERROR] deploy-backend.sh failed at line $LINENO — exit code $?\033[0m" >&2' ERR

# Configuration Defaults
APP_NAME="gostore"
APP_DIR="/opt/gostore"
USER="afari"
GROUP="www-data"
DEFAULT_INSTANCES=3
BASE_PORT=8080 # Instances will run on 8081, 8082, 8083, ...
DEFAULT_DOMAIN="file.proleadsolutions.co"

# Database Defaults
DB_TYPE="" # sqlite or postgres
DB_NAME=""
DB_USER=""
DB_PASSWORD=""

# Cloudflare Cache Purge (optional)
CF_ZONE_ID=""
CF_API_TOKEN=""

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Check if running as root
if [ "$EUID" -ne 0 ]; then 
    echo -e "${RED}Please run as root (use sudo bash deploy-backend.sh)${NC}"
    exit 1
fi

echo -e "${BLUE}================================================================${NC}"
echo -e "${CYAN}  GoStore / Prolead File - Production Server Deployment        ${NC}"
echo -e "${CYAN}  Target Domain: https://${DEFAULT_DOMAIN}                      ${NC}"
echo -e "${BLUE}================================================================${NC}"
echo ""

echo -e "${YELLOW}Phase 1: Cluster Sizing & System Toolchain${NC}"

# Ask for number of instances (3 to 5 recommended)
read -p "How many backend API worker instances do you want to run for load balancing? (e.g. 2, 3, or 5) [default: $DEFAULT_INSTANCES]: " NUM_INSTANCES
NUM_INSTANCES=${NUM_INSTANCES:-$DEFAULT_INSTANCES}

if ! [[ "$NUM_INSTANCES" =~ ^[0-9]+$ ]] || [ "$NUM_INSTANCES" -lt 1 ]; then
    echo -e "${RED}Invalid instance count. Defaulting to $DEFAULT_INSTANCES.${NC}"
    NUM_INSTANCES=$DEFAULT_INSTANCES
fi

echo -e "${GREEN}✓ Cluster configuration: $NUM_INSTANCES backend instances (Ports $(($BASE_PORT + 1)) to $(($BASE_PORT + $NUM_INSTANCES)))${NC}"

# Detect running user if afari doesn't exist
if ! id "$USER" &>/dev/null; then
    if [ -n "$SUDO_USER" ]; then
        USER="$SUDO_USER"
    else
        USER="gostore"
        echo -e "${YELLOW}Creating service user $USER...${NC}"
        useradd -m -s /bin/bash "$USER" || true
    fi
fi

# Ensure Go toolchain is installed
if ! command -v go &> /dev/null && [ ! -f "/usr/local/go/bin/go" ]; then
    echo -e "${YELLOW}Go is not installed. Installing Go 1.24...${NC}"
    wget -q https://go.dev/dl/go1.24.0.linux-amd64.tar.gz || wget -q https://go.dev/dl/go1.23.6.linux-amd64.tar.gz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf go*.linux-amd64.tar.gz
    rm -f go*.linux-amd64.tar.gz
    export PATH=$PATH:/usr/local/go/bin
    echo "export PATH=\$PATH:/usr/local/go/bin" >> /etc/profile
    echo -e "${GREEN}✓ Go installed successfully.${NC}"
else
    export PATH=$PATH:/usr/local/go/bin
    echo -e "${GREEN}✓ Go toolchain detected: $(go version)${NC}"
fi

# Ensure Node.js 22 LTS & npm (for Angular 22 web frontend)
NODE_VER=$(node -v 2>/dev/null | cut -d'.' -f1 | tr -d 'v' || echo "0")
if [ "$NODE_VER" -lt 22 ]; then
    echo -e "${YELLOW}Ensuring Node.js 22 LTS for Angular 22...${NC}"
    curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
    apt-get install -y -qq nodejs
    echo -e "${GREEN}✓ Node.js $(node -v) & npm $(npm -v) installed successfully.${NC}"
else
    echo -e "${GREEN}✓ Node.js $(node -v) detected.${NC}"
fi

# Ensure system packages
echo -e "${YELLOW}Ensuring system packages (git, curl, make, sqlite3, nginx, ufw, rsync)...${NC}"
apt-get update -qq
apt-get install -y -qq git curl make sqlite3 nginx rsync ufw

echo -e "${YELLOW}Phase 2: Directory Structure, Storage Engine & Permission Setup${NC}"

# Add service user to web server group for shared access
usermod -aG "$GROUP" "$USER" 2>/dev/null || true

# Explicitly create all required storage, data, log, and backup directories
mkdir -p "$APP_DIR"
mkdir -p "$APP_DIR/bin"
mkdir -p "$APP_DIR/data"
mkdir -p "$APP_DIR/data/storage"
mkdir -p "$APP_DIR/data/storage/.blobs"
mkdir -p "$APP_DIR/data/storage/temp"
mkdir -p "$APP_DIR/data/storage/trash"
mkdir -p "$APP_DIR/logs"
mkdir -p "$APP_DIR/backups"
mkdir -p "$APP_DIR/backups/postgres"
mkdir -p "$APP_DIR/config"

chmod 755 /opt "$APP_DIR"
chown -R $USER:$GROUP "$APP_DIR"

# Ensure read, write, and execute permissions on all runtime storage and log directories
chmod -R 775 "$APP_DIR/data"
chmod -R 775 "$APP_DIR/logs"
chmod -R 775 "$APP_DIR/backups"
chmod -R 775 "$APP_DIR/config"

echo -e "${GREEN}✓ Created storage directories with read/write permissions at $APP_DIR/data/storage${NC}"

echo -e "${YELLOW}Locating GoStore source repository...${NC}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ -f "$SCRIPT_DIR/cmd/server/main.go" ]; then
    SRC_DIR="$SCRIPT_DIR"
elif [ -d "/home/afari/Projects/file explorer" ] && [ -f "/home/afari/Projects/file explorer/cmd/server/main.go" ]; then
    SRC_DIR="/home/afari/Projects/file explorer"
else
    SRC_DIR=$(find /home /root /opt /var/www -maxdepth 4 -name "file explorer" -type d 2>/dev/null | while read -r d; do [ -f "$d/cmd/server/main.go" ] && echo "$d" && break; done || true)
    SRC_DIR=${SRC_DIR:-""}
fi

if [ -z "$SRC_DIR" ] || [ ! -f "$SRC_DIR/cmd/server/main.go" ]; then
    echo -e "${RED}Error: GoStore source directory not found.${NC}"
    echo -e "${YELLOW}Please place the source at $APP_DIR or run this script from the repository root.${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Found source at: $SRC_DIR${NC}"

# Synchronize code files while preserving production data, storage blobs, .env, and logs
rsync -a \
    --exclude='/.git' \
    --exclude='/.git/**' \
    --exclude='/bin' \
    --exclude='/bin/**' \
    --exclude='.env*' \
    --exclude='*.env' \
    --exclude='/data' \
    --exclude='/data/**' \
    --exclude='/storage' \
    --exclude='/storage/**' \
    --exclude='/logs' \
    --exclude='/logs/**' \
    --exclude='/backups' \
    --exclude='/backups/**' \
    --exclude='/nx_app' \
    --exclude='node_modules/' \
    --exclude='.angular/' \
    "$SRC_DIR/" "$APP_DIR/"

chown -R $USER:$GROUP "$APP_DIR" || true
chmod -R u+rwX,g+rX,o+rX "$APP_DIR" || true
chmod -R 775 "$APP_DIR/data" "$APP_DIR/logs" "$APP_DIR/backups" 2>/dev/null || true

cd "$APP_DIR"

echo -e "${YELLOW}Phase 2.5: PostgreSQL Production Database Setup${NC}"

DB_TYPE="postgres"
apt-get install -y -qq postgresql postgresql-contrib

echo -e "${YELLOW}Do you want to configure the PostgreSQL database and credentials?${NC}"
echo "   1) Yes - Setup database, user and grant permissions"
echo "   2) No  - Use existing PostgreSQL database & credentials"
read -p "   Enter choice [1-2, default 1]: " DB_CHOICE
DB_CHOICE=${DB_CHOICE:-1}

read -p "Enter PostgreSQL Database Name [default: $DB_NAME]: " INPUT_DB_NAME
DB_NAME=${INPUT_DB_NAME:-$DB_NAME}

read -p "Enter PostgreSQL Database User [default: $DB_USER]: " INPUT_DB_USER
DB_USER=${INPUT_DB_USER:-$DB_USER}

if [ "$DB_CHOICE" = "1" ]; then
    if [ -z "$DB_PASSWORD" ]; then
        read -s -p "Enter PostgreSQL Database Password: " DB_PASSWORD
        echo ""
    fi
    
    echo -e "${YELLOW}Configuring PostgreSQL database and user...${NC}"
    systemctl start postgresql
    systemctl enable postgresql
    
    sudo -u postgres psql -tc "SELECT 1 FROM pg_database WHERE datname = '$DB_NAME'" | grep -q 1 || \
        sudo -u postgres psql -c "CREATE DATABASE $DB_NAME;"
        
    sudo -u postgres psql -tc "SELECT 1 FROM pg_roles WHERE rolname = '$DB_USER'" | grep -q 1 || \
        sudo -u postgres psql -c "CREATE USER $DB_USER WITH ENCRYPTED PASSWORD '$DB_PASSWORD';"
        
    sudo -u postgres psql -c "ALTER USER $DB_USER WITH ENCRYPTED PASSWORD '$DB_PASSWORD';"
    sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE $DB_NAME TO $DB_USER;"
    sudo -u postgres psql -c "ALTER DATABASE $DB_NAME OWNER TO $DB_USER;"
    echo -e "${GREEN}✓ PostgreSQL database '$DB_NAME' configured successfully${NC}"
fi

# Automated PostgreSQL Backup Configuration (Daily at 2 AM UTC)
BACKUP_DIR="/opt/gostore/backups/postgres"
mkdir -p "$BACKUP_DIR"
chown -R postgres:postgres /opt/gostore/backups 2>/dev/null || true

cat << 'BACKUP_SCRIPT' > "$APP_DIR/backup_db.sh"
#!/bin/bash
DB_NAME="REPLACE_DB_NAME"
BACKUP_DIR="/opt/gostore/backups/postgres"
DATE=$(date +"%Y%m%d_%H%M%S")
RETENTION_DAYS=30
BACKUP_FILE="${BACKUP_DIR}/${DB_NAME}_${DATE}.sql.gz"

mkdir -p "$BACKUP_DIR"
sudo -u postgres pg_dump "$DB_NAME" | gzip > "$BACKUP_FILE"
find "$BACKUP_DIR" -name "*.sql.gz" -type f -mtime +$RETENTION_DAYS -delete
echo "[$(date)] GoStore PostgreSQL backup completed: $BACKUP_FILE"
BACKUP_SCRIPT
sed -i "s/REPLACE_DB_NAME/$DB_NAME/g" "$APP_DIR/backup_db.sh"
chmod +x "$APP_DIR/backup_db.sh"
(crontab -l 2>/dev/null | grep -v "backup_db.sh"; echo "0 2 * * * $APP_DIR/backup_db.sh >> $APP_DIR/logs/backup.log 2>&1") | crontab -
echo -e "${GREEN}✓ Automated daily PostgreSQL backup configured (02:00 AM UTC)${NC}"

echo -e "${YELLOW}Phase 3: Environment Configuration${NC}"

if [ ! -f "$APP_DIR/.env" ]; then
    if [ -f "$SRC_DIR/.env.example" ]; then
        echo -e "${YELLOW}Creating .env from .env.example...${NC}"
        cp "$SRC_DIR/.env.example" "$APP_DIR/.env"
    elif [ -f "$SRC_DIR/.env" ]; then
        echo -e "${YELLOW}Copying existing .env...${NC}"
        cp "$SRC_DIR/.env" "$APP_DIR/.env"
    fi
    chown $USER:$GROUP "$APP_DIR/.env"
    chmod 600 "$APP_DIR/.env"
    
    # Configure production environment variables in .env
    sed -i "s|^ENV=.*|ENV=production|g" "$APP_DIR/.env" || echo "ENV=production" >> "$APP_DIR/.env"
    sed -i "s|^DB_TYPE=.*|DB_TYPE=postgres|g" "$APP_DIR/.env"
    sed -i "s|^POSTGRES_DB=.*|POSTGRES_DB=$DB_NAME|g" "$APP_DIR/.env"
    sed -i "s|^POSTGRES_USER=.*|POSTGRES_USER=$DB_USER|g" "$APP_DIR/.env"
    if [ -n "$DB_PASSWORD" ]; then
        sed -i "s|^POSTGRES_PASSWORD=.*|POSTGRES_PASSWORD=$DB_PASSWORD|g" "$APP_DIR/.env"
    fi
    echo -e "${CYAN}Please review settings in $APP_DIR/.env${NC}"
else
    echo -e "${GREEN}✓ Existing .env preserved at $APP_DIR/.env${NC}"
    chmod 600 "$APP_DIR/.env"
fi

echo -e "${YELLOW}Phase 4: Direct Compilation (Go & Angular Frontend)${NC}"

export PATH=$PATH:/usr/local/go/bin

echo -e "${YELLOW}Building Angular 22 Studio Web App...${NC}"
if [ -d "$APP_DIR/frontend" ] && [ -f "$APP_DIR/frontend/package.json" ]; then
    sudo -u $USER bash -c "cd '$APP_DIR/frontend' && (npm ci --legacy-peer-deps || npm install --legacy-peer-deps) && npm run build"
    echo -e "${GREEN}✓ Web frontend built and copied to internal static assets.${NC}"
fi

mkdir -p "$APP_DIR/bin"
chown -R $USER:$GROUP "$APP_DIR/bin" 2>/dev/null || true

echo -e "${YELLOW}Downloading Go dependencies...${NC}"
sudo -u $USER bash -c "cd '$APP_DIR' && env PATH=\"\$PATH:/usr/local/go/bin\" go mod tidy"

if [ -f "$APP_DIR/cmd/server/main.go" ]; then
    echo -e "${YELLOW}Compiling GoStore Server (cmd/server/main.go)...${NC}"
    sudo -u $USER bash -c "cd '$APP_DIR' && env PATH=\"\$PATH:/usr/local/go/bin\" go build -ldflags='-s -w' -o bin/server ./cmd/server/"
    chmod +x "$APP_DIR/bin/server"
    echo -e "${GREEN}✓ GoStore Server compiled: $APP_DIR/bin/server${NC}"
fi

if [ -f "$APP_DIR/cmd/createsuperuser/main.go" ]; then
    echo -e "${YELLOW}Compiling Superuser Creator (cmd/createsuperuser/main.go)...${NC}"
    sudo -u $USER bash -c "cd '$APP_DIR' && env PATH=\"\$PATH:/usr/local/go/bin\" go build -ldflags='-s -w' -o bin/createsuperuser ./cmd/createsuperuser/"
    chmod +x "$APP_DIR/bin/createsuperuser"
    echo -e "${GREEN}✓ Superuser Creator CLI compiled: $APP_DIR/bin/createsuperuser${NC}"
fi

if [ -f "$APP_DIR/cmd/prolead-cli/main.go" ]; then
    echo -e "${YELLOW}Compiling Prolead File CLI (cmd/prolead-cli/main.go)...${NC}"
    sudo -u $USER bash -c "cd '$APP_DIR' && env PATH=\"\$PATH:/usr/local/go/bin\" go build -ldflags='-s -w' -o bin/prolead-cli ./cmd/prolead-cli/"
    chmod +x "$APP_DIR/bin/prolead-cli"
    echo -e "${GREEN}✓ Prolead File CLI compiled: $APP_DIR/bin/prolead-cli${NC}"
fi

# Admin User Account Creation Prompt
echo ""
echo -e "${YELLOW}Do you want to create or update a GoStore Admin Account now?${NC}"
echo "   1) Yes - Run Admin Account Creator"
echo "   2) No  - Skip admin creation"
read -p "   Enter choice [1-2, default 2]: " ADMIN_CHOICE
ADMIN_CHOICE=${ADMIN_CHOICE:-2}

if [ "$ADMIN_CHOICE" = "1" ]; then
    read -p "Enter Admin Username [default: admin]: " ADMIN_USER
    ADMIN_USER=${ADMIN_USER:-admin}
    read -p "Enter Admin Email [default: admin@proleadsolutions.co]: " ADMIN_EMAIL
    ADMIN_EMAIL=${ADMIN_EMAIL:-admin@proleadsolutions.co}
    read -s -p "Enter Admin Password [default: Admin123!]: " ADMIN_PASS
    ADMIN_PASS=${ADMIN_PASS:-Admin123!}
    echo ""
    
    sudo -u $USER bash -c "cd '$APP_DIR' && env PATH=\"\$PATH\" \"$APP_DIR/bin/createsuperuser\" \
      -username=\"$ADMIN_USER\" \
      -email=\"$ADMIN_EMAIL\" \
      -password=\"$ADMIN_PASS\"" || echo -e "${YELLOW}Admin creation step completed.${NC}"
fi

echo -e "${YELLOW}Phase 5: Systemd Multi-Instance Cluster Setup${NC}"

mkdir -p "$APP_DIR/config"
chown -R $USER:$GROUP "$APP_DIR/config"

# 1. Template Unit File
cat << 'EOF' > /etc/systemd/system/gostore@.service
[Unit]
Description=GoStore / Prolead File Server Instance #%i
After=network.target

[Service]
Type=simple
User=APP_USER
Group=APP_GROUP
WorkingDirectory=APP_DIR_PATH
EnvironmentFile=APP_DIR_PATH/.env
EnvironmentFile=APP_DIR_PATH/config/instance-%i.env
ExecStart=APP_DIR_PATH/bin/server
Restart=always
RestartSec=3
StandardOutput=journal
StandardError=journal
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

sed -i "s|APP_USER|$USER|g" /etc/systemd/system/gostore@.service
sed -i "s|APP_GROUP|$GROUP|g" /etc/systemd/system/gostore@.service
sed -i "s|APP_DIR_PATH|$APP_DIR|g" /etc/systemd/system/gostore@.service

# 2. Master Cluster Target File
WANTS_LIST=""
for ((i=1; i<=NUM_INSTANCES; i++)); do
    WANTS_LIST="$WANTS_LIST gostore@$i.service"
done

cat << EOF > /etc/systemd/system/gostore.target
[Unit]
Description=GoStore / Prolead File Backend Cluster Target
Wants=$WANTS_LIST

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload

# 3. Create per-instance env configs and start all configured instances
echo -e "${YELLOW}Starting $NUM_INSTANCES GoStore instances...${NC}"
for ((i=1; i<=NUM_INSTANCES; i++)); do
    INSTANCE_PORT=$(($BASE_PORT + $i))
    cat << ENV_EOF > "$APP_DIR/config/instance-$i.env"
PORT=$INSTANCE_PORT
ENV_EOF
    chown $USER:$GROUP "$APP_DIR/config/instance-$i.env"
    chmod 600 "$APP_DIR/config/instance-$i.env"

    systemctl enable gostore@$i
    systemctl restart gostore@$i
    echo -e "${GREEN}  ✓ Instance #$i active on Port $INSTANCE_PORT${NC}"
done

# Stop any excess instances if reducing count
for ((i=NUM_INSTANCES+1; i<=20; i++)); do
    if systemctl is-active --quiet gostore@$i; then
        systemctl stop gostore@$i || true
        systemctl disable gostore@$i || true
    fi
done

systemctl enable gostore.target
systemctl restart gostore.target
echo -e "${GREEN}✓ All $NUM_INSTANCES GoStore instances running and managed by gostore.target${NC}"

echo -e "${YELLOW}Phase 6: Nginx Load Balancer & SSL Gateway Configuration${NC}"

read -p "Do you want to configure Nginx Load Balancer with SSL for $DEFAULT_DOMAIN? (y/n) [default: y]: " SETUP_SSL
SETUP_SSL=${SETUP_SSL:-y}

# Generate Upstream Servers list
UPSTREAM_SERVERS=""
UPSTREAM_WS_SERVERS=""
for ((i=1; i<=NUM_INSTANCES; i++)); do
    PORT=$(($BASE_PORT + $i))
    UPSTREAM_SERVERS="${UPSTREAM_SERVERS}    server 127.0.0.1:${PORT} max_fails=3 fail_timeout=10s;\n"
    UPSTREAM_WS_SERVERS="${UPSTREAM_WS_SERVERS}    server 127.0.0.1:${PORT};\n"
done

if [ "$SETUP_SSL" = "y" ]; then
    read -p "Enter your GoStore Domain [default: $DEFAULT_DOMAIN]: " API_DOMAIN
    API_DOMAIN=${API_DOMAIN:-$DEFAULT_DOMAIN}
    
    mkdir -p "/etc/nginx/ssl/$API_DOMAIN"
    
    if [ ! -f "/etc/nginx/ssl/$API_DOMAIN/cert.pem" ]; then
        echo "Please paste your SSL Certificate (Cloudflare Origin Cert or Let's Encrypt fullchain.pem), then press Ctrl+D:"
        cat > "/etc/nginx/ssl/$API_DOMAIN/cert.pem"
    else
        echo -e "${GREEN}✓ SSL Certificate found at /etc/nginx/ssl/$API_DOMAIN/cert.pem${NC}"
    fi
    
    if [ ! -f "/etc/nginx/ssl/$API_DOMAIN/key.pem" ]; then
        echo "Please paste your SSL Private Key (key.pem or privkey.pem), then press Ctrl+D:"
        cat > "/etc/nginx/ssl/$API_DOMAIN/key.pem"
    else
        echo -e "${GREEN}✓ SSL Private Key found at /etc/nginx/ssl/$API_DOMAIN/key.pem${NC}"
    fi
    
    chmod 600 "/etc/nginx/ssl/$API_DOMAIN/key.pem"
    
    cat << EOF > /etc/nginx/sites-available/$APP_NAME
# ------------------------------------------------------------------------------
# GoStore Nginx Load Balancer Pool: $NUM_INSTANCES Instances (Ports $(($BASE_PORT + 1)) - $(($BASE_PORT + $NUM_INSTANCES)))
# ------------------------------------------------------------------------------
upstream gostore_cluster {
    least_conn;
$(echo -e "$UPSTREAM_SERVERS")
    keepalive 64;
}

upstream gostore_ws_cluster {
    ip_hash;
$(echo -e "$UPSTREAM_WS_SERVERS")
}

# Rate limiting zones
limit_req_zone \$binary_remote_addr zone=gostore_api_limit:10m rate=100r/s;
limit_req_zone \$binary_remote_addr zone=gostore_auth_limit:10m rate=20r/s;

server {
    listen 80;
    server_name $API_DOMAIN;
    server_tokens off;
    
    # Drop direct IP scans
    if (\$host ~* "^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$") {
        return 444;
    }
    
    return 301 https://\$host\$request_uri;
}

server {
    listen 443 ssl http2;
    server_name $API_DOMAIN;
    server_tokens off;

    # Drop direct IP scans
    if (\$host ~* "^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$") {
        return 444;
    }

    ssl_certificate /etc/nginx/ssl/$API_DOMAIN/cert.pem;
    ssl_certificate_key /etc/nginx/ssl/$API_DOMAIN/key.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers on;
    ssl_ciphers 'ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:DHE-RSA-AES128-GCM-SHA256:DHE-RSA-AES256-GCM-SHA384';

    # Security Headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains; preload" always;

    # High-Throughput & Resumable TUS Chunk Uploads (unlimited / large payload)
    client_max_body_size 0;
    proxy_request_buffering off;

    # Rate-limited Auth Endpoints
    location /api/v1/auth/ {
        limit_req zone=gostore_auth_limit burst=30 nodelay;
        proxy_pass http://gostore_cluster;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Connection "";
    }

    # Real-Time SSE & WebSockets (No proxy buffering, long timeout)
    location ~* /(events|ws|sse)/ {
        proxy_pass http://gostore_ws_cluster;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 86400s;
        proxy_send_timeout 86400s;
    }

    # Main Application & API Proxy
    location / {
        limit_req zone=gostore_api_limit burst=150 nodelay;
        proxy_pass http://gostore_cluster;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "";
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_cache_bypass \$http_upgrade;

        proxy_connect_timeout 15s;
        proxy_send_timeout 300s;
        proxy_read_timeout 300s;
    }
}
EOF
else
    cat << EOF > /etc/nginx/sites-available/$APP_NAME
upstream gostore_cluster {
    least_conn;
$(echo -e "$UPSTREAM_SERVERS")
    keepalive 64;
}

server {
    listen 80;
    server_name _;
    server_tokens off;

    client_max_body_size 0;
    proxy_request_buffering off;

    location / {
        proxy_pass http://gostore_cluster;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "";
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_cache_bypass \$http_upgrade;
    }
}
EOF
fi

ln -sf /etc/nginx/sites-available/$APP_NAME /etc/nginx/sites-enabled/
nginx -t && systemctl reload nginx || systemctl restart nginx
echo -e "${GREEN}✓ Nginx Load Balancer configured for $NUM_INSTANCES GoStore instances${NC}"

# Optional Cloudflare Cache Purge
if [ -n "$CF_ZONE_ID" ] && [ -n "$CF_API_TOKEN" ]; then
    echo -e "${YELLOW}Purging Cloudflare cache...${NC}"
    CF_RESPONSE=$(curl -s -X POST "https://api.cloudflare.com/client/v4/zones/$CF_ZONE_ID/purge_cache" \
        -H "Authorization: Bearer $CF_API_TOKEN" \
        -H "Content-Type: application/json" \
        --data '{"purge_everything":true}')
    if echo "$CF_RESPONSE" | grep -q '"success":true'; then
        echo -e "${GREEN}✓ Cloudflare cache purged successfully${NC}"
    else
        echo -e "${YELLOW}⚠ Cloudflare purge status: $CF_RESPONSE${NC}"
    fi
fi

echo -e "${YELLOW}Phase 7: Network Firewall (UFW) Hardening${NC}"
read -p "Do you want to configure and enable UFW firewall (allow 22, 80, 443; isolate internal backend ports)? (y/n) [default: y]: " SETUP_UFW
SETUP_UFW=${SETUP_UFW:-y}
if [ "$SETUP_UFW" = "y" ]; then
    ufw default deny incoming
    ufw default allow outgoing
    ufw allow 22/tcp comment 'SSH'
    ufw allow 80/tcp comment 'HTTP'
    ufw allow 443/tcp comment 'HTTPS'
    ufw --force enable
    echo -e "${GREEN}✓ UFW Firewall enabled: ports 22, 80, 443 open.${NC}"
fi

echo ""
echo -e "${BLUE}================================================================${NC}"
echo -e "${GREEN}✓ GoStore Production Deployment Completed Successfully!        ${NC}"
echo -e "${BLUE}================================================================${NC}"
echo -e "${CYAN}Production URL:   ${NC} https://$DEFAULT_DOMAIN"
echo -e "${CYAN}Active Instances: ${NC} $NUM_INSTANCES backend instances (Ports $(($BASE_PORT + 1)) to $(($BASE_PORT + $NUM_INSTANCES)))"
echo -e "${CYAN}Cluster Status:   ${NC} sudo systemctl status gostore.target"
echo -e "${CYAN}Single Instance:  ${NC} sudo systemctl status gostore@1"
echo -e "${CYAN}Cluster Logs:     ${NC} sudo journalctl -u 'gostore@*' -f"
echo -e "${CYAN}App Directory:    ${NC} $APP_DIR"
echo -e "${CYAN}Storage Engine:   ${NC} $APP_DIR/data/storage"
echo ""
