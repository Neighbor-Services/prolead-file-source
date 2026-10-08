#!/bin/bash
# ==============================================================================
# GoStore / Prolead File — Teardown & Clean Decommission Script
# Usage: sudo bash teardown-backend.sh
# ==============================================================================

set -e

RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Please run as root (sudo bash teardown-backend.sh)${NC}"
    exit 1
fi

echo -e "${YELLOW}================================================================${NC}"
echo -e "${YELLOW}  GoStore / Prolead File — Service Teardown                     ${NC}"
echo -e "${YELLOW}================================================================${NC}"
read -p "Are you sure you want to stop and disable all GoStore services? (y/n): " CONFIRM
if [ "$CONFIRM" != "y" ]; then
    echo "Aborted."
    exit 0
fi

echo -e "${YELLOW}Stopping systemd services...${NC}"
systemctl stop gostore.target 2>/dev/null || true
systemctl disable gostore.target 2>/dev/null || true

for inst in $(systemctl list-units --type=service --all 2>/dev/null | grep -oE "gostore@[0-9]+" | sort -u); do
    systemctl stop "$inst" 2>/dev/null || true
    systemctl disable "$inst" 2>/dev/null || true
done

rm -f /etc/systemd/system/gostore@.service /etc/systemd/system/gostore.target
systemctl daemon-reload

echo -e "${YELLOW}Disabling Nginx site...${NC}"
rm -f /etc/nginx/sites-enabled/gostore
nginx -t 2>/dev/null && systemctl reload nginx || true

echo -e "${GREEN}✓ GoStore services have been cleanly stopped and disabled.${NC}"
echo -e "${CYAN}Note: Application files and data at /opt/gostore were preserved.${NC}"
