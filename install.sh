#!/bin/bash
set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${BLUE}==> Welcome to AgentLens Installer${NC}"

# 1. Check for dependencies
if ! command -v docker &> /dev/null; then
    echo -e "${RED}Error: Docker is not installed. Please install Docker first.${NC}"
    exit 1
fi

if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
    echo -e "${RED}Error: docker-compose is not installed. Please install it first.${NC}"
    exit 1
fi

if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed. AgentLens needs Go to compile the CA generator.${NC}"
    exit 1
fi

# 2. Build and generate certificates
echo -e "${BLUE}==> Generating local SSL certificates for MITM interception...${NC}"
make setup-certs || true

# 3. Spin up the backend with Docker Compose
echo -e "${BLUE}==> Starting AgentLens services via Docker...${NC}"
if command -v docker-compose &> /dev/null; then
    make deploy
else
    # Fallback to `docker compose` plugin if `docker-compose` standalone isn't found
    docker compose up -d --build
fi

# 4. Create the CLI wrapper script
LENS_SCRIPT=$(mktemp)
cat << 'EOF' > "$LENS_SCRIPT"
#!/bin/bash
if [ -z "$1" ]; then
    echo -e "\033[0;34mAgentLens Wrapper\033[0m"
    echo "Usage: lens <agent_command>"
    echo "Example: lens claude"
    exit 1
fi

# Temporarily inject proxy env vars for this process only
export HTTP_PROXY="http://localhost:8080"
export HTTPS_PROXY="http://localhost:8080"
export NODE_EXTRA_CA_CERTS="$HOME/.agentlens/ca.crt"
export REQUESTS_CA_BUNDLE="$HOME/.agentlens/ca.crt"
export SSL_CERT_FILE="$HOME/.agentlens/ca.crt"

# Execute the agent securely
exec "$@"
EOF

chmod +x "$LENS_SCRIPT"

# 5. Install the wrapper script
INSTALL_PATH="/usr/local/bin/lens"
LOCAL_BIN="$HOME/.local/bin"
LOCAL_PATH="$LOCAL_BIN/lens"

echo -e "${BLUE}==> Installing 'lens' wrapper command...${NC}"
if [ -w "/usr/local/bin" ]; then
    mv "$LENS_SCRIPT" "$INSTALL_PATH"
    echo -e "${GREEN}Successfully installed to $INSTALL_PATH${NC}"
else
    echo "Requires sudo to install to /usr/local/bin. Prompting for password..."
    if sudo -n true 2>/dev/null || sudo mv "$LENS_SCRIPT" "$INSTALL_PATH" 2>/dev/null; then
        echo -e "${GREEN}Successfully installed to $INSTALL_PATH${NC}"
    else
        echo "Sudo failed or requires interactive password. Falling back to ~/.local/bin..."
        mkdir -p "$LOCAL_BIN"
        mv "$LENS_SCRIPT" "$LOCAL_PATH"
        echo -e "${GREEN}Successfully installed to $LOCAL_PATH${NC}"
        if [[ ":$PATH:" != *":$LOCAL_BIN:"* ]]; then
            echo -e "${RED}Warning: $LOCAL_BIN is not in your PATH. Please add it to your ~/.zshrc or ~/.bashrc.${NC}"
        fi
    fi
fi

echo -e "\n${GREEN}🎉 AgentLens installed successfully!${NC}"
echo -e "Dashboard is live at: ${BLUE}http://localhost:8090${NC}"
echo -e "To trace an agent, simply prepend it with ${BLUE}lens${NC}:"
echo -e "  $ lens claude"
echo -e "  $ lens agy"
echo -e "\nHappy tracing!"
