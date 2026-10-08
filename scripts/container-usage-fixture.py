#!/usr/bin/env python3
"""Synthetic loopback upstream for the disposable container acceptance test."""
import json
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


class Fixture(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        usage = {'prompt_tokens': 3, 'completion_tokens': 2, 'total_tokens': 5}
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream' if body.get('stream') else 'application/json')
        self.send_header('Connection', 'close')
        self.end_headers()
        if body.get('stream'):
            payload = 'data: ' + json.dumps({'choices': [], 'usage': usage}) + '\n\ndata: [DONE]\n\n'
        else:
            payload = json.dumps({'choices': [{'message': {'role': 'assistant', 'content': 'fixture'}}], 'usage': usage})
        raw = payload.encode()
        for offset in range(0, len(raw), 7):
            self.wfile.write(raw[offset:offset + 7])
            self.wfile.flush()
        self.close_connection = True


server = ThreadingHTTPServer(('0.0.0.0', 0), Fixture)
Path(sys.argv[1]).write_text(str(server.server_port))
server.serve_forever()
