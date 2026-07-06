import requests
import os
import time

GATEWAY_URL = os.environ.get("AGENTLENS_GATEWAY", "http://localhost:8080")
GEMINI_KEY = os.environ.get("GEMINI_API_KEY", "dummy-key")

def test_missing_session_header():
    """Test 10A: Missing X-AgentLens-Session-ID header gracefully falls back to auto-generated session"""
    url = f"{GATEWAY_URL}/proxy/gemini/v1beta/models/gemini-3.5-flash:generateContent?key={GEMINI_KEY}"
    
    # Missing session ID header
    headers = {
        "Content-Type": "application/json",
        "X-AgentLens-Agent-ID": "test-bot"
    }
    
    resp = requests.post(url, json={"contents": [{"role": "user", "parts": [{"text": "Hello"}]}]}, headers=headers)
    assert resp.status_code in [200, 400, 429]
    # We can't assert the session in the DB easily since we don't know the auto-generated ID,
    # but asserting it didn't crash (500) is the goal here.

def test_missing_agent_header(agentlens):
    """Test 10B: Missing X-AgentLens-Agent-ID header defaults to 'unknown'"""
    url = f"{GATEWAY_URL}/proxy/gemini/v1beta/models/gemini-3.5-flash:generateContent?key={GEMINI_KEY}"
    
    headers = {
        "Content-Type": "application/json",
        "X-AgentLens-Session-ID": agentlens.session_id
    }
    
    requests.post(url, json={"contents": [{"role": "user", "parts": [{"text": "Hello"}]}]}, headers=headers)
    time.sleep(1)
    
    sess = agentlens.get_session()
    assert sess["agent_id"] == "unknown"


def test_invalid_api_key(agentlens):
    """Test 11A: Invalid API Key passes the error gracefully to client"""
    url = f"{GATEWAY_URL}/proxy/gemini/v1beta/models/gemini-3.5-flash:generateContent?key=invalid-key-123"
    
    headers = {
        "Content-Type": "application/json",
        "X-AgentLens-Session-ID": agentlens.session_id,
        "X-AgentLens-Agent-ID": "test-bot"
    }
    
    resp = requests.post(url, json={"contents": [{"role": "user", "parts": [{"text": "Hello"}]}]}, headers=headers)
    assert resp.status_code == 400 # Gemini returns 400 for bad key (API_KEY_INVALID)
    
    time.sleep(1)
    sess = agentlens.get_session()
    assert sess["id"] == agentlens.session_id
