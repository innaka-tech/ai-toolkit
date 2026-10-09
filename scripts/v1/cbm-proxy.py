#!/usr/bin/env python3
"""HTTP reverse proxy: rewrite Host header so codebase-memory UI (localhost-only)
is reachable from WG peers via 10.10.7.3:9749."""
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import http.client, sys

UPSTREAM_HOST = "127.0.0.1"
UPSTREAM_PORT = 9749

class Proxy(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def _proxy(self):
        length = int(self.headers.get("Content-Length", 0) or 0)
        body = self.rfile.read(length) if length else None

        conn = http.client.HTTPConnection(UPSTREAM_HOST, UPSTREAM_PORT, timeout=30)
        # Rewrite Host header to what the UI server accepts
        headers = {k: v for k, v in self.headers.items() if k.lower() not in ("host", "connection")}
        headers["Host"] = "127.0.0.1"
        headers["Connection"] = "close"
        try:
            conn.request(self.command, self.path, body=body, headers=headers)
            resp = conn.getresponse()
            data = resp.read()
        except Exception as e:
            self.send_error(502, f"upstream error: {e}")
            return
        finally:
            conn.close()

        self.send_response(resp.status)
        for k, v in resp.getheaders():
            if k.lower() in ("transfer-encoding", "connection", "content-length", "content-encoding"):
                continue
            self.send_header(k, v)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    do_GET = _proxy
    do_POST = _proxy
    do_DELETE = _proxy
    do_OPTIONS = _proxy
    do_PUT = _proxy
    do_PATCH = _proxy
    do_HEAD = _proxy

    def log_message(self, fmt, *args):
        print(f"[{self.address_string()}] {self.command} {self.path} -> {self.responses.get(self.status, ('',''))[0] if hasattr(self,'status') else ''}", flush=True)

srv = ThreadingHTTPServer(("0.0.0.0", 9749), Proxy)
print(f"http-proxy listening on 0.0.0.0:9749 -> {UPSTREAM_HOST}:{UPSTREAM_PORT} (Host rewritten)", flush=True)
srv.serve_forever()
