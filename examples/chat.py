import os
import sys
import uuid
import time
import requests
import json

AGENTLENS_GATEWAY = os.environ.get("AGENTLENS_GATEWAY", "http://localhost:8080")
GEMINI_API_KEY = os.environ.get("GEMINI_API_KEY")

if not GEMINI_API_KEY:
    print("❌ Error: GEMINI_API_KEY environment variable is missing.")
    print("Run with: GEMINI_API_KEY='your-key' python3 chat.py")
    sys.exit(1)

session_id = str(uuid.uuid4())
agent_id = "interactive-demo-bot"
model = "gemini-3.5-flash"
url = f"{AGENTLENS_GATEWAY}/proxy/gemini/v1beta/models/{model}:generateContent?key={GEMINI_API_KEY}"

print(f"🔗 Using AgentLens Gateway: {AGENTLENS_GATEWAY}")
print(f"📝 Session ID: {session_id}")
print("\n" + "="*60)
print(" 🚀 INTERACTIVE AGENTLENS DEMO ")
print("="*60)
print("Type your message and press Enter. Type 'quit' to exit.")
print("\n💡 Tip 1: To trigger FRUSTRATION, type in ALL CAPS or say 'YOU ARE USELESS!'")
print("💡 Tip 2: To trigger HALLUCINATION & INFRA ERROR, type '/fail'")
print("="*60)

history = []

while True:
    try:
        user_input = input("\n🧑 You: ")
    except (KeyboardInterrupt, EOFError):
        break

    if user_input.strip().lower() in ['quit', 'exit']:
        break
    if not user_input.strip():
        continue

    simulate_failure = False
    if user_input.strip() == '/fail':
        simulate_failure = True
        user_input = "Fetch the production logs for the API service."
        print(f"🔄 Intercepted command. Sending prompt: '{user_input}'")

    history.append({"role": "user", "parts": [{"text": user_input}]})

    payload = {
        "contents": history
    }
    
    headers = {
        "Content-Type": "application/json",
        "X-AgentLens-Session-ID": session_id,
        "X-AgentLens-Agent-ID": agent_id,
        "X-AgentLens-User-Message": user_input
    }

    start_time = time.time()
    resp = requests.post(url, json=payload, headers=headers)
    latency = int((time.time() - start_time) * 1000)
    
    if resp.status_code != 200:
        print("❌ API Error:", resp.text)
        history.pop() # remove failed message
        continue

    data = resp.json()
    try:
        model_reply = data["candidates"][0]["content"]["parts"][0]["text"].strip()
    except Exception:
        model_reply = "Error parsing response."
        
    print(f"\n🤖 Gemini ({latency}ms): {model_reply}")
    
    # Append model reply to history
    history.append({"role": "model", "parts": [{"text": model_reply}]})

    # Simulate tool execution
    if simulate_failure:
        print("🛠️  Simulating a FAILING database tool execution...")
        # Passing an invalid upstream URL forces AgentLens to record an Infra Error and a Tool Failure
        requests.post(f"{AGENTLENS_GATEWAY}/tools/execute", json={
            "session_id": session_id,
            "turn_id": "",
            "tool_name": "db_query",
            "category": "database",
            "params": {"query": "SELECT * FROM prod_logs"},
            "upstream_url": "http://127.0.0.1:9999/timeout"
        })
    else:
        # Simulate a successful generic search tool just to have data
        requests.post(f"{AGENTLENS_GATEWAY}/tools/execute", json={
            "session_id": session_id,
            "turn_id": "",
            "tool_name": "web_search",
            "category": "http",
            "params": {"query": user_input},
            "upstream_url": "" # Empty means success in mock mode
        })

print("\n👋 Session ended. Check Dashboard at http://localhost:8090 to see the timeline, frustration spikes, and errors!")
