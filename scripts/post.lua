wrk.method = "POST"
wrk.body   = '{"type": "order.created", "data": {"order_id": "1234"}}'
wrk.headers["Content-Type"] = "application/json"
wrk.headers["X-API-Key"] = "kestrel-dev-key"
