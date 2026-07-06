import os
import uuid
import time
import requests
import pytest

GATEWAY_URL = os.environ.get("AGENTLENS_GATEWAY", "http://localhost:8080")
API_URL = os.environ.get("AGENTLENS_API", "http://localhost:8090/api/v1")
GEMINI_KEY = os.environ.get("GEMINI_API_KEY", "dummy-key-for-testing")

@pytest.fixture
def session_id():
    """Returns a unique session ID for testing."""
    return str(uuid.uuid4())

@pytest.fixture
def agent_id():
    """Returns a test agent ID."""
    return "pytest-runner"

class AgentLensClient:
    def __init__(self, session_id, agent_id):
        self.session_id = session_id
        self.agent_id = agent_id
        self.history = []
        
    def chat(self, message):
        """Sends a message to the mocked proxy to get a response (and record the turn)."""
        self.history.append({"role": "user", "parts": [{"text": message}]})
        url = f"{GATEWAY_URL}/proxy/gemini/v1beta/models/gemini-3.5-flash:generateContent?key={GEMINI_KEY}"
        
        headers = {
            "Content-Type": "application/json",
            "X-AgentLens-Session-ID": self.session_id,
            "X-AgentLens-Agent-ID": self.agent_id,
            "X-AgentLens-User-Message": message
        }
        
        # We'll just mock the response by hitting the gateway, but since we might not have a real key,
        # we could also just send to an invalid model and expect a 400. But the gateway records it!
        # Wait, if we send a dummy key, Gemini returns 400. AgentLens still records the turn!
        # But we need a valid model_response for hallucination detection.
        # Let's hit the proxy but point it to a mock upstream if needed, OR just use the real Gemini API if key is present.
        
        resp = requests.post(url, json={"contents": self.history}, headers=headers)
        return resp

    def log_tool(self, tool_name, category, params, upstream_url=""):
        """Logs a tool execution to AgentLens."""
        resp = requests.post(f"{GATEWAY_URL}/tools/execute", json={
            "session_id": self.session_id,
            "turn_id": "",
            "tool_name": tool_name,
            "category": category,
            "params": params,
            "upstream_url": upstream_url
        })
        return resp

    def get_session(self):
        """Fetches the session data from the Dashboard API."""
        return requests.get(f"{API_URL}/sessions/{self.session_id}").json()
        
    def get_turns(self):
        """Fetches the turns for this session from the Dashboard API."""
        return requests.get(f"{API_URL}/sessions/{self.session_id}/turns").json()

    def get_timeline(self):
        """Fetches the full timeline for this session."""
        return requests.get(f"{API_URL}/sessions/{self.session_id}/timeline").json()


@pytest.fixture
def agentlens(session_id, agent_id):
    """Provides a helper client to interact with AgentLens."""
    return AgentLensClient(session_id, agent_id)

