#!/usr/bin/env sh
# Приёмник уведомлений Alertmanager для скриншота (ДЗ №6).
#
# Зачем: для сдачи нужен скриншот ПОЛУЧЕННОГО уведомления. Telegram требует
# bot token и chat_id — не нужны, если показать фактический POST от Alertmanager.
# Скрипт печатает тело уведомления в терминал: это и есть доказательство доставки.
#
# Alertmanager может стоять в кластере, а приёмник — на хосте. Тогда:
#   kubectl -n monitoring port-forward svc/alertmanager 9093:9093 &
#   ./slo/webhook-sink.sh &
# и в alertmanager.yml url заменить на http://host.minikube.internal:9095/alerts
#
# Зависимости: только python3 (stdlib), ничего ставить не нужно.

set -eu

PORT="${PORT:-9095}"

exec python3 - "$PORT" <<'PY'
import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

PORT = int(sys.argv[1])


class Sink(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        raw = self.rfile.read(length)

        print("\n" + "=" * 70)
        print(f"ALERTMANAGER POST  {self.path}")
        print("=" * 70)

        try:
            payload = json.loads(raw)
        except json.JSONDecodeError:
            print(raw.decode("utf-8", "replace"))
        else:
            print(f"status : {payload.get('status')}")
            print(f"group  : {payload.get('groupLabels')}")
            for a in payload.get("alerts", []):
                labels = a.get("labels", {})
                ann = a.get("annotations", {})
                print("-" * 70)
                print(f"  {labels.get('alertname')}  [{labels.get('severity')}]")
                print(f"  status   : {a.get('status')}")
                print(f"  summary  : {ann.get('summary')}")
                print(f"  startsAt : {a.get('startsAt')}")
                if a.get("endsAt") and a.get("endsAt") != "0001-01-01T00:00:00Z":
                    print(f"  endsAt   : {a.get('endsAt')}")

        sys.stdout.flush()
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b'{"ok":true}')

    def log_message(self, *args):
        pass  # своя печать выше, access-лог не нужен


print(f"webhook sink listening on :{PORT}  (POST /alerts)")
HTTPServer(("", PORT), Sink).serve_forever()
PY
