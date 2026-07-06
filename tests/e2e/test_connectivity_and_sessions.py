import time

def test_proxy_connectivity(agentlens):
    """Test 1: Route a simple call through the proxy and confirm it works."""
    resp = agentlens.chat("Hello!")
    
    # Depending on the GEMINI_API_KEY, this might be 200 or 400. 
    # But as long as it's not a connection error (e.g. 502), the proxy is working!
    assert resp.status_code in [200, 400], f"Proxy failed with status {resp.status_code}"

    # Wait for async DB write
    time.sleep(1)

    # Check dashboard API to ensure it was logged
    sess = agentlens.get_session()
    assert sess["id"] == agentlens.session_id
    assert sess["provider"] == "gemini"


def test_session_tracking(agentlens):
    """Test 2: Send 5 messages in same session, verify single session creation."""
    for i in range(5):
        agentlens.chat(f"Message {i}")
        time.sleep(0.5)

    time.sleep(1)

    sess = agentlens.get_session()
    assert sess["turn_count"] == 5

    turns = agentlens.get_turns()
    assert len(turns) == 5
    
    # Ensure they are in order
    indexes = [t["index"] for t in turns]
    assert indexes == sorted(indexes)
