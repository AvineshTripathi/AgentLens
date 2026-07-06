import os
import sys
import uuid
import time
import requests
import argparse

# ─── Configuration ────────────────────────────────────────────────────────
AGENTLENS_GATEWAY = os.environ.get("AGENTLENS_GATEWAY", "http://localhost:8080")
GEMINI_API_KEY = os.environ.get("GEMINI_API_KEY")

def main():
    parser = argparse.ArgumentParser(description="AgentLens Gemini Integration Demo")
    parser.add_argument("--prompt", type=str, default="Tell me a very short joke about logging.", help="The prompt to send to Gemini")
    parser.add_argument("--simulate-tool", action="store_true", help="Simulate a tool execution along with the turn")
    args = parser.parse_args()

    if not GEMINI_API_KEY:
        print("❌ Error: GEMINI_API_KEY environment variable is missing.")
        print("Run with: GEMINI_API_KEY='your-key' python3 gemini_agent.py")
        sys.exit(1)

    print(f"🔗 Using AgentLens Gateway: {AGENTLENS_GATEWAY}")
    
    # 1. Setup a unique Session ID for tracking in the Dashboard
    session_id = str(uuid.uuid4())
    agent_id = "gemini-demo-bot"
    print(f"📝 Session ID: {session_id}")
    
    # 2. Build the Gemini Request
    # Note how we swap 'https://generativelanguage.googleapis.com' for our Gateway URL + '/proxy/gemini'
    model = "gemini-3.5-flash"
    url = f"{AGENTLENS_GATEWAY}/proxy/gemini/v1beta/models/{model}:generateContent?key={GEMINI_API_KEY}"
    
    payload = {
        "contents": [{
            "parts": [{"text": args.prompt}]
        }]
    }

    # Inject AgentLens tracking headers
    headers = {
        "Content-Type": "application/json",
        "X-AgentLens-Session-ID": session_id,
        "X-AgentLens-Agent-ID": agent_id,
        "X-AgentLens-User-Message": args.prompt # Helps with Frustration Analysis
    }

    print("\n🚀 Sending request to Gemini via AgentLens...")
    start_time = time.time()
    
    response = requests.post(url, json=payload, headers=headers)
    
    if response.status_code != 200:
        print(f"❌ API Error: {response.status_code}")
        print(response.text)
        sys.exit(1)

    latency = int((time.time() - start_time) * 1000)
    data = response.json()
    
    # Extract Gemini's response text
    try:
        model_reply = data["candidates"][0]["content"]["parts"][0]["text"].strip()
    except (KeyError, IndexError):
        model_reply = "Could not parse response text."

    print(f"✅ Received response in {latency}ms")
    print(f"🤖 Gemini: {model_reply}\n")

    # 3. (Optional) Log a tool execution to AgentLens
    if args.simulate_tool:
        print("🛠️  Simulating a backend tool execution (e.g., querying a database)...")
        # In real life, you'd actually execute the tool here
        tool_result = {"status": "success", "rows": 5, "data": "dummy data"}
        
        tool_payload = {
            "session_id": session_id,
            "turn_id": "",
            "tool_name": "db_query",
            "category": "database",
            "params": {"query": "SELECT * FROM users"},
            "upstream_url": "" # Leave blank if executed locally in Python
        }
        
        tool_resp = requests.post(f"{AGENTLENS_GATEWAY}/tools/execute", json=tool_payload)
        if tool_resp.status_code == 200:
            print("✅ Tool execution successfully logged to AgentLens!")
        else:
            print(f"⚠️ Failed to log tool execution: {tool_resp.text}")

    print("\n🎯 Done! Check your AgentLens Dashboard at http://localhost:8090 to see this session.")

if __name__ == "__main__":
    main()
