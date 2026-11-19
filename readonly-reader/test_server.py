import hashlib
import json
import sqlite3
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


class ReaderTest(unittest.TestCase):
    def test_stdio_and_scope(self):
        with tempfile.TemporaryDirectory() as directory:
            db = Path(directory) / "messages.db"
            with sqlite3.connect(db) as c:
                c.execute("CREATE TABLE messages(id TEXT, chat_jid TEXT, sender TEXT, content TEXT, timestamp TEXT, is_from_me INTEGER, deleted_at TEXT)")
                c.executemany("INSERT INTO messages VALUES(?,?,?,?,?,?,?)", [
                    ("1", "allow", "sender", "test-visible", "2026-01-01", 0, None),
                    ("2", "other", "sender", "test-private", "2026-01-02", 0, None),
                    ("3", "allow", "sender", "test-deleted", "2026-01-03", 0, "2026-01-04"),
                ])
            c.close()
            before = hashlib.sha256(db.read_bytes()).digest()
            methods = [
                ("initialize", {"protocolVersion": "2025-11-25"}),
                ("tools/list", {}),
                ("tools/call", {"name": "list_recent_messages", "arguments": {"chat_jid": "allow"}}),
                ("tools/call", {"name": "list_recent_messages", "arguments": {"chat_jid": "other"}}),
                ("tools/call", {"name": "send_message", "arguments": {}}),
                ("tools/call", {"name": "list_recent_messages", "arguments": {"chat_jid": "allow", "limit": 500}}),
            ]
            request = "".join(json.dumps({"jsonrpc": "2.0", "id": i, "method": m, "params": p}) + "\n" for i, (m, p) in enumerate(methods))
            result = subprocess.run([sys.executable, str(Path(__file__).with_name("server.py")), "--db", str(db), "--allow-chat", "allow"], input=request, text=True, capture_output=True, check=True)
            self.assertEqual(result.stderr, "")
            res = [json.loads(line) for line in result.stdout.splitlines()]
            self.assertEqual(res[0]["result"]["protocolVersion"], "2025-11-25")
            self.assertEqual([v["name"] for v in res[1]["result"]["tools"]], ["list_recent_messages"])
            self.assertEqual([v["content"] for v in json.loads(res[2]["result"]["content"][0]["text"])["messages"]], ["test-visible"])
            self.assertTrue(res[3]["result"]["isError"])
            self.assertEqual(res[4]["error"]["code"], -32601)
            self.assertTrue(res[5]["result"]["isError"])
            self.assertEqual(hashlib.sha256(db.read_bytes()).digest(), before)


if __name__ == "__main__":
    unittest.main()
