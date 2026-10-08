#!/bin/bash
# ==============================================================================
# GoStore / Prolead File — Services Restart & Status Script
#
# Usage:
#   sudo bash restart-services.sh          (Restart GoStore cluster & reload Nginx)
#   sudo bash restart-services.sh --status (Check status of all instances)
#   sudo bash restart-services.sh --nginx  (Reload Nginx only)
# ==============================================================================

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

echo -e "${BLUE}================================================================${NC}"
echo -e "${CYAN}   GoStore / Prolead File — Service Manager                     ${NC}"
echo -e "${BLUE}================================================================${NC}"
echo ""

IS_ROOT=false
if [ "$EUID" -eq 0 ]; then
    IS_ROOT=true
fi

MODE="${1:-all}"

status_check() {
    echo -e "${YELLOW}▶ Checking GoStore Cluster Status...${NC}"
    if [ "$IS_ROOT" = true ]; then
        systemctl status gostore.target --no-pager || true
        echo ""
        for inst in $(systemctl list-units --type=service --all 2>/dev/null | grep -oE "gostore@[0-9]+" | sort -u); do
            systemctl status "$inst" --no-pager -n 3 || true
            echo ""
        done
    else
        ps aux | grep -E "bin/server|gostore" | grep -v grep || echo "No local GoStore processes running."
    fi
}

restart_gostore() {
    echo -e "${YELLOW}▶ Restarting GoStore Multi-Instance Cluster...${NC}"
    if [ "$IS_ROOT" = true ]; then
        if systemctl list-unit-files | grep -q "gostore.target"; then
            systemctl restart gostore.target && echo -e "${GREEN}  ✓ gostore.target restarted${NC}" || true
        fi
        for inst in $(systemctl list-units --type=service --all 2>/dev/null | grep -oE "gostore@[0-9]+" | sort -u); do
            echo -e "  Restarting instance $inst..."
            systemctl restart "$inst" && echo -e "${GREEN}  ✓ $inst restarted${NC}" || true
        done
    else
        echo -e "${CYAN}  Checking local running GoStore processes...${NC}"
        pkill -f "./bin/server" 2>/dev/null && echo -e "${GREEN}  ✓ Stopped local GoStore binary${NC}" || true
        pkill -f "go run ./cmd/server/main.go" 2>/dev/null && echo -e "${GREEN}  ✓ Stopped local go run process${NC}" || true
    fi
}

reload_nginx() {
    echo -e "${YELLOW}▶ Verifying and Reloading Nginx...${NC}"
    if [ "$IS_ROOT" = true ] && command -v nginx &>/dev/null; then
        if nginx -t 2>/dev/null; then
            systemctl reload nginx && echo -e "${GREEN}  ✓ Nginx reloaded successfully${NC}" || systemctl restart nginx
        else
            echo -e "${RED}  ✗ Nginx syntax test failed; skipping reload${NC}"
        fi
    fi
}

case "$MODE" in
    --status)
        status_check
        ;;
    --nginx)
        reload_nginx
        ;;
    *)
        restart_gostore
        reload_nginx
        ;;
esac

echo ""
echo -e "${BLUE}================================================================${NC}"
echo -e "${GREEN}  ✓ Operation completed!                                        ${NC}"
echo -e "${BLUE}================================================================${NC}"
