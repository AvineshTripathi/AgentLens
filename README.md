# AgentLens

AgentLens is a full-stack observability platform designed to give you complete visibility into AI agent behavior. It acts as an intelligent HTTP proxy and middleware that intercepts LLM calls and tool executions, detecting hallucinations, tracing infrastructure errors, and measuring user frustration.

## Features

- **Agent Proxy Gateway:** Intercepts traffic to Anthropic, OpenAI, and Gemini.
- **Hallucination Detection:** Uses zero-latency heuristics to flag contradictions between model claims and actual tool results.
- **Frustration Analyzer:** Scores user frustration based on behavioral (rage prompting) and linguistic signals.
- **Infra Correlator:** Maps database timeouts and API failures to the exact conversation turn where output quality degraded.
- **Real-time Dashboard:** A built-in, dark-mode dashboard (served from the Go binary) showing a complete timeline of every session.
- **Prometheus Metrics:** Pre-configured metrics for tokens, latency, frustration events, and tool errors.

## Running Locally

You can run the entire AgentLens stack (AgentLens, PostgreSQL, Prometheus, Grafana) locally using Docker Compose.

### 1. Start the stack
Run the following command in the root of the project:
```bash
docker compose up -d
```
This will start:
- **AgentLens Gateway:** `http://localhost:8080`
- **AgentLens Dashboard:** `http://localhost:8090`
- **PostgreSQL:** `localhost:5432` (User: `agentlens`, Pass: `agentlens`)
- **Prometheus:** `http://localhost:9090`
- **Grafana:** `http://localhost:3000` (User: `admin`, Pass: `admin`)

### 2. Generate Mock Traffic
To see the dashboard in action without having to hook up a real agent, run the provided mock traffic script. This will simulate a user having a frustrating interaction with an agent:

```bash
chmod +x scripts/mock_traffic.sh
./scripts/mock_traffic.sh
```

### 3. View the Dashboard
Open your browser and navigate to:
**http://localhost:8090**

You will see the live metrics, the frustration funnel, hallucination heatmap, and a detailed timeline view for the session that was just created.

## Integration

To integrate your own agent with AgentLens, simply point your agent's API base URL to the AgentLens gateway, and pass in the necessary headers.

### Example: OpenAI
```bash
export OPENAI_API_BASE="http://localhost:8080/proxy/openai/v1"
```

### Example: Anthropic (via curl)
```bash
curl -X POST http://localhost:8080/proxy/anthropic/v1/messages \
  -H "x-api-key: your-api-key" \
  -H "X-AgentLens-Session-ID: my-session-123" \
  -H "X-AgentLens-Agent-ID: my-cli-agent" \
  -H "X-AgentLens-User-Message: Hello world" \
  ...
```

For tool executions, ensure your agent framework hits the `/tools/execute` endpoint before and after running a tool to log the attempt and outcome.

### Example: Gemini Python Demo
We have included a full python demo in the `examples/` directory.

1. Ensure AgentLens is running (`docker compose up -d`)
2. Install dependencies: `pip install requests`
3. Run the demo:
```bash
GEMINI_API_KEY="your-real-api-key" python3 examples/gemini_agent.py \
  --prompt "Write a haiku about kubernetes" \
  --simulate-tool
```
This script will route traffic to Gemini via AgentLens and optionally simulate a tool execution so you can see it in the dashboard.
