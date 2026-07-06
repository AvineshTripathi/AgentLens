#!/bin/bash
# Simulates AI agent traffic to the AgentLens gateway for local testing.

GATEWAY_URL="http://localhost:8080"
SESSION_ID=$(uuidgen)
TURN_ID=$(uuidgen)

echo "🚀 Simulating an Agent session..."
echo "Session ID: $SESSION_ID"

# 1. Send a User-Agent interaction (Turn 1: Success)
echo "→ Sending Turn 1: Normal user query..."
curl -s -X POST "$GATEWAY_URL/proxy/anthropic/v1/messages" \
  -H "Content-Type: application/json" \
  -H "X-AgentLens-Session-ID: $SESSION_ID" \
  -H "X-AgentLens-Agent-ID: cli-agent" \
  -H "X-AgentLens-User-Message: Fetch me the latest logs for the API service." \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "messages": [{"role": "user", "content": "Fetch me the latest logs for the API service."}],
    "max_tokens": 1024
  }' > /dev/null

sleep 1

# 2. Simulate a tool call (success)
echo "→ Simulating Tool Call: Fetch Logs..."
curl -s -X POST "$GATEWAY_URL/tools/execute" \
  -H "Content-Type: application/json" \
  -d "{
    \"session_id\": \"$SESSION_ID\",
    \"turn_id\": \"$TURN_ID\",
    \"tool_name\": \"log_search\",
    \"category\": \"compute\",
    \"params\": {\"service\": \"api\", \"limit\": 100},
    \"upstream_url\": \"\"
  }" > /dev/null

sleep 1

# 3. Send Turn 2: User getting frustrated + Hallucination signal
# We simulate a hallucination by making the user complain about a made-up action
echo "→ Sending Turn 2: Frustrated user (triggering hallucination checks)..."
curl -s -X POST "$GATEWAY_URL/proxy/anthropic/v1/messages" \
  -H "Content-Type: application/json" \
  -H "X-AgentLens-Session-ID: $SESSION_ID" \
  -H "X-AgentLens-Agent-ID: cli-agent" \
  -H "X-AgentLens-User-Message: NO, you misunderstood! I said production logs. YOU GAVE ME GARBAGE!" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "messages": [{"role": "user", "content": "NO, you misunderstood! I said production logs. YOU GAVE ME GARBAGE!"}],
    "max_tokens": 1024
  }' > /dev/null

sleep 1

# 4. Simulate a failing tool call (Infra error)
echo "→ Simulating Tool Call: Database Query (Failing)..."
curl -s -X POST "$GATEWAY_URL/tools/execute" \
  -H "Content-Type: application/json" \
  -d "{
    \"session_id\": \"$SESSION_ID\",
    \"turn_id\": \"$TURN_ID\",
    \"tool_name\": \"postgres_query\",
    \"category\": \"database\",
    \"params\": {\"query\": \"SELECT * FROM logs\"},
    \"upstream_url\": \"http://nonexistent.local\"
  }" > /dev/null

sleep 1

# 5. Send Turn 3: User abandons session
echo "→ Sending Turn 3: User abandonment..."
curl -s -X POST "$GATEWAY_URL/proxy/anthropic/v1/messages" \
  -H "Content-Type: application/json" \
  -H "X-AgentLens-Session-ID: $SESSION_ID" \
  -H "X-AgentLens-Agent-ID: cli-agent" \
  -H "X-AgentLens-User-Message: Forget it, this is useless. I will do it myself." \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "messages": [{"role": "user", "content": "Forget it, this is useless. I will do it myself."}],
    "max_tokens": 1024
  }' > /dev/null

sleep 1

# 6. Close the session
echo "→ Closing Session with outcome: abandoned"
curl -s -X POST "$GATEWAY_URL/sessions/$SESSION_ID/close" \
  -H "Content-Type: application/json" \
  -d '{"outcome": "abandoned"}' > /dev/null

echo "✅ Simulation complete!"
echo "Check your dashboard at http://localhost:8090 to see the results."
