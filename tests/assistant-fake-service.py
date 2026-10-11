"""Deterministic, loopback-only Assistant protocol fixture; no external APIs."""
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

TOKEN = "synthetic_assistant_vm_token_" + "x" * 32
TURN = "00000000-0000-4000-8000-000000000001"
ACCOUNT = {"name": "VM laboratory", "plan": "trial", "status": "active", "turnsRemaining": 5, "periodEnd": "2099-12-31"}
LOG = Path("/tmp/fake-requests.log")


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_args):
        pass

    def record(self, body):
        has_text = any(m.get("role") == "user" and m.get("content") for m in body.get("messages", []))
        with LOG.open("a") as output:
            output.write(json.dumps({"method": self.command, "path": self.path, "userText": bool(has_text)}) + "\n")

    def respond(self, status, body, content_type="application/json"):
        data = body.encode() if isinstance(body, str) else json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def error(self, status, code):
        self.respond(status, {"error": {"code": code, "message": "Synthetic fixture refusal"}})

    def authenticated(self):
        if self.headers.get("Authorization") == "Bearer " + TOKEN:
            return True
        self.error(401, "invalid_token")
        return False

    def do_GET(self):
        self.record({})
        if self.path == "/v1/offer":
            self.respond(200, {"priceText": "360 € + VAT per lab / year", "trialRequests": 5, "trialUrl": "https://example.invalid/trial", "subscribeUrl": "https://example.invalid/assistant/"})
            return
        if self.authenticated():
            if self.path == "/v1/account":
                self.respond(200, {"account": ACCOUNT})
            else:
                self.error(404, "turn_not_found")

    def do_POST(self):
        data = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        body = json.loads(data) if data else {}
        self.record(body)
        if self.path == "/v1/activate":
            if body.get("code") != "TEST-CODE-0001":
                self.error(400, "invalid_code")
            else:
                self.respond(200, {"token": TOKEN, "account": ACCOUNT})
            return
        if not self.authenticated():
            return
        if self.path == "/v1/turns":
            self.respond(201, {"turnId": TURN, "limits": {"steps": 24, "seconds": 900, "maxRequestBytes": 2000000}})
        elif self.path == f"/v1/turns/{TURN}/cancel":
            self.respond(204, "")
        elif self.path == f"/v1/turns/{TURN}/steps":
            results = {m["toolCallId"]: json.loads(m["content"]) for m in body["messages"] if m["role"] == "tool"}
            if "write-course" not in results:
                event, payload = "tool_calls", {"calls": [{"id": "write-course", "name": "write_file", "arguments": {"path": "modules/course.nix", "content": "{ course = 2; }\n", "executable": False}}]}
            elif "evaluate-course" not in results:
                assert results["write-course"]["ok"]
                event, payload = "tool_calls", {"calls": [{"id": "evaluate-course", "name": "evaluate", "arguments": {}}]}
            else:
                assert results["evaluate-course"] == {"ok": True, "changedHosts": ["pc01"]}
                event, payload = "final", {"summary": "Prepared the course for pc01."}
            self.respond(200, f"event: {event}\ndata: {json.dumps(payload)}\n\n", "text/event-stream")
        else:
            self.error(404, "turn_not_found")


if __name__ == "__main__":
    LOG.write_text("")
    ThreadingHTTPServer(("127.0.0.1", 18480), Handler).serve_forever()
