import sys
import time
import requests
import concurrent.futures
import random
import json

# ==========================================
# Kestrel Interview Traffic Simulator
# ==========================================

if len(sys.argv) < 3:
    print("Usage: python interview_traffic.py <API_KEY> <BACKEND_URL>")
    print("Example: python interview_traffic.py kestrel_abc123 https://kestrel-api-xyz.onrender.com")
    sys.exit(1)

API_KEY = sys.argv[1]
BASE_URL = sys.argv[2].rstrip('/')

HEADERS = {
    "X-API-Key": API_KEY,
    "Content-Type": "application/json"
}

EVENTS_TO_SEND = 500
CONCURRENCY = 10

def send_event(event_id):
    payload = {
        "type": "user.signed_up",
        "payload": {
            "user_id": f"usr_{random.randint(1000, 9999)}",
            "email": f"demo_{event_id}@example.com",
            "plan": "premium"
        }
    }
    
    try:
        start = time.time()
        res = requests.post(f"{BASE_URL}/api/events", headers=HEADERS, json=payload, timeout=5)
        latency = (time.time() - start) * 1000
        
        if res.status_code == 201:
            sys.stdout.write(f"\r[SUCCESS] Sent event {event_id}/{EVENTS_TO_SEND} [{latency:.0f}ms]")
            sys.stdout.flush()
        elif res.status_code == 429:
            sys.stdout.write(f"\r[RATE LIMITED] Event {event_id} (Sliding Window active)!")
            sys.stdout.flush()
        else:
            sys.stdout.write(f"\r[ERROR] {res.status_code} - {res.text}")
            sys.stdout.flush()
    except Exception as e:
        pass

print(f"[START] Starting Kestrel Traffic Simulation...")
print(f"[TARGET] {BASE_URL}")
print(f"[INFO] Simulating {EVENTS_TO_SEND} webhook events across {CONCURRENCY} concurrent threads...")
print("-" * 50)

with concurrent.futures.ThreadPoolExecutor(max_workers=CONCURRENCY) as executor:
    # We add a tiny random sleep to make the dashboard graphs look realistic and organic
    futures = []
    for i in range(1, EVENTS_TO_SEND + 1):
        futures.append(executor.submit(send_event, i))
        time.sleep(random.uniform(0.01, 0.05))

print("\n\n[DONE] Traffic simulation complete! Check your React Dashboard.")
