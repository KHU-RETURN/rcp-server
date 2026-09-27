"""Run the note function behind a small local HTTP and SQLite mock."""

import json
import sqlite3
import subprocess
import tempfile
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path


HERE = Path(__file__).resolve().parent
ROOT = HERE.parent.parent
PREFIX = "/api/v1/run/00000000-0000-0000-0000-000000000001"
DATABASE = sqlite3.connect(":memory:")
DATABASE.execute("CREATE TABLE notes (id TEXT PRIMARY KEY, text TEXT NOT NULL, created_at TEXT NOT NULL)")


def data_reply(request):
    if request.get("$rcp") != "sql" or request.get("binding") != "DB":
        return {"$rcp": "sql.result", "ok": False, "error": "invalid binding"}
    try:
        cursor = DATABASE.execute(request["sql"], request.get("params") or [])
        columns = [item[0] for item in cursor.description] if cursor.description else []
        rows = [dict(zip(columns, row)) for row in cursor.fetchall()] if columns else []
        affected = cursor.rowcount if cursor.rowcount > 0 else 0
        DATABASE.commit()
        return {"$rcp": "sql.result", "ok": True, "columns": columns,
                "rows": rows, "rows_affected": affected}
    except sqlite3.Error as error:
        return {"$rcp": "sql.result", "ok": False, "error": str(error)}


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.handle_request()

    def do_POST(self):
        self.handle_request()

    def do_DELETE(self):
        self.handle_request()

    def send_json(self, status, payload):
        body = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def handle_request(self):
        if not self.path.startswith(PREFIX + "/"):
            if self.command != "GET":
                return self.send_json(404, {"error": "not found"})
            filename = self.path.lstrip("/") or "index.html"
            if filename not in {"index.html", "style.css", "app.js"}:
                return self.send_json(404, {"error": "not found"})
            content_type = {"index.html": "text/html", "style.css": "text/css", "app.js": "text/javascript"}[filename]
            body = (HERE / filename).read_bytes()
            self.send_response(200)
            self.send_header("Content-Type", content_type + "; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            return self.wfile.write(body)

        if self.headers.get("Authorization") != "Bearer local-demo":
            return self.send_json(401, {"error": "invalid function key"})
        size = int(self.headers.get("Content-Length", "0"))
        if size > 65536:
            return self.send_json(413, {"error": "request body too large"})
        body = self.rfile.read(size).decode("utf-8")
        event = {"method": self.command, "path": self.path[len(PREFIX):], "body": body}
        process = subprocess.Popen(
            [self.server.function_binary], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
            stderr=subprocess.PIPE, text=True, bufsize=1,
        )
        try:
            process.stdin.write(json.dumps(event) + "\n")
            process.stdin.flush()
            line = process.stdout.readline()
            message = json.loads(line)
            if message.get("$rcp") == "sql":
                process.stdin.write(json.dumps(data_reply(message)) + "\n")
                process.stdin.flush()
                message = json.loads(process.stdout.readline())
            process.stdin.close()
            process.wait(timeout=5)
            if process.returncode != 0:
                return self.send_json(502, {"error": "function exited with an error"})
            response = message["body"].encode()
            self.send_response(message["statusCode"])
            self.send_header("Content-Type", message["headers"]["content-type"])
            self.send_header("Content-Length", str(len(response)))
            self.end_headers()
            self.wfile.write(response)
        except (ValueError, KeyError, subprocess.TimeoutExpired, OSError):
            process.kill()
            self.send_json(502, {"error": "invalid function response"})


if __name__ == "__main__":
    with tempfile.TemporaryDirectory(prefix="rcp-notes-demo-") as directory:
        binary = str(Path(directory) / "notes")
        subprocess.run(["go", "build", "-o", binary, "./examples/serverless-notes"], cwd=ROOT, check=True)
        server = HTTPServer(("127.0.0.1", 8000), Handler)
        server.function_binary = binary
        print("Open http://127.0.0.1:8000", flush=True)
        server.serve_forever()
