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

## Installation (One-Click)

AgentLens comes with an automated installer that will set up Docker Compose, generate your local SSL certificates, and install the `lens` CLI wrapper.

```bash
./install.sh
```

This will automatically spin up:
- **AgentLens Gateway:** `http://localhost:8080`
- **AgentLens Dashboard:** `http://localhost:8090`
- **PostgreSQL:** `localhost:5432` 

Open your browser and navigate to **http://localhost:8090** to view the dashboard!

## Usage (`lens` wrapper)

Instead of manually exporting global proxy variables and polluting your terminal environment, AgentLens installs a lightweight wrapper command called `lens`. 

Simply prepend `lens` to whatever agent command you want to run. It dynamically injects the proxy settings **only** for that specific execution, ensuring that all other background terminal traffic remains completely unaffected.

```bash
lens claude
# or
lens agy
# or
lens python my_agent.py
```

## Architecture Overview
AgentLens intercepts traffic asynchronously to prevent adding latency to the user's terminal experience:
1. The Agent CLI sends an HTTP request to the Gateway.
2. The Gateway computes a session ID (via headers or payload hashing) and forwards the request to the real LLM provider.
3. The response is buffered and sent back to the CLI immediately.
4. Asynchronously, the Gateway parses the buffered payload, extracts token usage, tool calls, and text, and persists it to PostgreSQL.

## Configuration

You can configure the upstream LLM endpoints by editing the `config.yaml` file located in the root directory. For example, to change your Gemini endpoint:

```yaml
proxy:
  gemini_upstream: "https://your-custom-endpoint.com"
```

After modifying the file, simply run `make deploy` again to rebuild the image and apply the changes.
