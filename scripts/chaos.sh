#!/bin/bash
echo "=== Starting Chaos Test ==="
echo "Waiting 5 seconds for benchmark to build up steam..."
sleep 5

echo "[CHAOS] Killing Redis..."
docker compose -f deployments/docker-compose.yml kill redis
sleep 5

echo "[CHAOS] Starting Redis..."
docker compose -f deployments/docker-compose.yml start redis
sleep 5

echo "[CHAOS] Killing PostgreSQL..."
docker compose -f deployments/docker-compose.yml kill postgres
sleep 10

echo "[CHAOS] Starting PostgreSQL..."
docker compose -f deployments/docker-compose.yml start postgres
sleep 5

echo "[CHAOS] Killing Kestrel (Worker node crash)..."
docker compose -f deployments/docker-compose.yml kill kestrel
sleep 5

echo "[CHAOS] Starting Kestrel..."
docker compose -f deployments/docker-compose.yml start kestrel

echo "=== Chaos Events Completed ==="
echo "Waiting for benchmark to finish..."
