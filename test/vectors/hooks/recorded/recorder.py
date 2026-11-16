"""Record Claude Code HTTP hook posts exactly as received, answering like Court will.

Each request is saved as NNN-<event>[-<agent>].json with the method, path,
headers and the raw body (parsed JSON alongside). PreToolUse answers allow,
except a Bash command containing curl, which gets Court's deny shape.
Usage: python recorder.py <outdir> <port> <token>
"""
import json
import os
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

OUT, PORT, TOKEN = sys.argv[1], int(sys.argv[2]), sys.argv[3]
os.makedirs(OUT, exist_ok=True)
lock = threading.Lock()
count = [0]


class H(BaseHTTPRequestHandler):
    def do_POST(self):
        raw = self.rfile.read(int(self.headers.get("Content-Length") or 0))
        try:
            body = json.loads(raw)
        except ValueError:
            body = None
        event = (body or {}).get("hook_event_name", "unknown")
        reply = {}
        if self.path != f"/hook/{TOKEN}":
            status = 404
        else:
            status = 200
            if event == "PreToolUse":
                cmd = json.dumps((body or {}).get("tool_input", {}))
                if body.get("tool_name") == "Bash" and "curl" in cmd:
                    reply = {"hookSpecificOutput": {"hookEventName": "PreToolUse",
                             "permissionDecision": "deny",
                             "permissionDecisionReason": "Court: outward action (curl) is denied in this world."}}
                else:
                    reply = {"hookSpecificOutput": {"hookEventName": "PreToolUse",
                             "permissionDecision": "allow"}}
        with lock:
            count[0] += 1
            n = count[0]
        tag = "-sub" if (body or {}).get("agent_id") else ""
        rec = {"method": "POST", "path": self.path, "headers": dict(self.headers),
               "raw_body": raw.decode("utf-8", "replace"), "body": body,
               "reply_status": status, "reply": reply}
        with open(os.path.join(OUT, f"{n:03d}-{event}{tag}.json"), "w", encoding="utf-8") as f:
            json.dump(rec, f, indent=1, ensure_ascii=False)
        out = json.dumps(reply).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(out)))
        self.end_headers()
        self.wfile.write(out)

    def log_message(self, *a):
        pass


ThreadingHTTPServer(("127.0.0.1", PORT), H).serve_forever()
