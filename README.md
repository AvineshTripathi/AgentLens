# AgentLens

AgentLens is a full-stack, passive observability platform designed to give you complete visibility into AI CLI agent behavior. 

Instead of requiring you to embed SDKs into your agent's code, AgentLens operates as a **Man-in-the-Middle (MITM) reverse proxy**. It passively intercepts API calls made by the agent CLI to the underlying LLM provider, extracting session data, tokens, latency, and tool executions transparently.

AgentLens natively supports:
- **Claude Code** (Anthropic)
- **Antigravity CLI / agy** (Google Gemini)

## Features

- **Zero-Code Integration:** Works by simply proxying HTTP traffic via environment variables. No SDK required.
- **Native Claude Session Sync:** Automatically extracts Claude Code's internal CLI session IDs to perfectly sync your terminal sessions with the dashboard.
- **Hallucination Detection:** Flag contradictions between model claims and actual tool results.
- **Frustration Analyzer:** Scores user frustration based on behavioral (e.g. rage prompting) and linguistic signals.
- **Real-time Dashboard:** A built-in dashboard showing a complete timeline of every session.

## Running Locally

You can run the entire AgentLens stack locally using Docker Compose.

### 1. Start the stack
Run the following command in the root of the project:
```bash
docker compose up -d
```
This will start:
- **AgentLens Gateway:** `http://localhost:8080`
- **AgentLens Dashboard:** `http://localhost:8090`
- **PostgreSQL:** `localhost:5432` 

### 2. View the Dashboard
Open your browser and navigate to:
**http://localhost:8090**

## How to Use (Routing Traffic)

To use AgentLens with your CLI agents, you can route all traffic through the proxy globally by setting standard proxy environment variables and telling your runtimes to trust the AgentLens certificate.

```bash
export HTTP_PROXY="http://localhost:8080"
export HTTPS_PROXY="http://localhost:8080"
export NODE_EXTRA_CA_CERTS="/Users/avinesh/.agentlens/ca.crt"
export REQUESTS_CA_BUNDLE="/Users/avinesh/.agentlens/ca.crt"

# Now run your agent normally!
claude
# or
agy
```

## Architecture Overview
AgentLens intercepts traffic asynchronously to prevent adding latency to the user's terminal experience:
1. The Agent CLI sends an HTTP request to the Gateway.
2. The Gateway computes a session ID (via headers or payload hashing) and forwards the request to the real LLM provider.
3. The response is buffered and sent back to the CLI immediately.
4. Asynchronously, the Gateway parses the buffered payload, extracts token usage, tool calls, and text, and persists it to PostgreSQL.
5. Background Python workers scan the database for complex patterns (e.g., hallucination heuristics).

For a deep dive into the system's design, see `architecture.md` (internal).
