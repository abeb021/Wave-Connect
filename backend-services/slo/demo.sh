#!/usr/bin/env sh
# Демонстрация изменения SLI/SLO/SLA при деградации Wave Connect.
#
# Требует: kubectl, curl. vmalert НЕ требуется — все запросы есть в slo/query.txt.
#
# Использование:
#   ./slo/demo.sh panels     # проверить, что метрики доехали до VM
#   ./slo/demo.sh traffic    # генерировать «нормальный» трафик
#   ./slo/demo.sh break-feed # уронить feed-service (scale 0) и слать трафик
#   ./slo/demo.sh break-db   # feed жив, но БД недоступна (500 изнутри)
#   ./slo/demo.sh break-auth # уронить auth-service
#   ./slo/demo.sh heal       # вернуть всё назад
#   ./slo/demo.sh urls       # запросы для vmui

set -eu

GATEWAY="${GATEWAY:-http://127.0.0.1:8080}"
NS="${NS:-default}"
DURATION="${DURATION:-90}"
RPS="${RPS:-2}"

email="demo$(date +%s)@example.com"
pass="password"
TOKEN=""
warned=""

# preflight — убедиться, что gateway вообще отвечает. Без этого
# скрипт молотил бы 90 секунд в пустоту.
preflight() {
  # curl при недоступном хосте возвращает ненулевой код — при `set -e`
  # это оборвёт скрипт, поэтому глушим его явно.
  code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 "$GATEWAY/" 2>/dev/null) || true
  case "$code" in
  '' | 000*) conn_failed=1 ;;
  *) conn_failed=0 ;;
  esac
  if [ "$conn_failed" = 1 ]; then
    cat >&2 <<EOF
!! gateway недоступен: $GATEWAY

Проверь по шагам:
  1) minikube status            # должен быть Running
  2) kubectl get pods           # поды должны быть Running
  3) kubectl port-forward svc/gateway-service 8080:8080 &

Если minikube не стартует из-за docker.sock — добавь себя в группу:
  sudo usermod -aG docker \$USER && newgrp docker
EOF
    exit 1
  fi
}

login() {
  curl -s -o /dev/null -D - -X POST "$GATEWAY/api/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"$email\",\"password\":\"$pass\"}" 2>/dev/null \
    | tr -d '\r' | awk -F': ' 'tolower($1)=="authorization"{print $2}'
}

ensure_user() {
  curl -s -o /dev/null -X POST "$GATEWAY/api/auth/register" \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"$email\",\"password\":\"$pass\"}" || true
  TOKEN="$(login)"
  if [ -z "$TOKEN" ] && [ -z "$warned" ]; then
    warned=1
    echo "!! нет токена — логин не удался (проверь: curl -i -X POST $GATEWAY/api/auth/login)" >&2
  fi
  if [ -n "$TOKEN" ]; then
    warned=""
  fi
}

traffic() {
  # Три «слоя» запросов: auth (login), feed (list), chat (conversation).
  # Каждый цикл — 3 запроса; ~RPS циклов в секунду.
  i=0
  while [ "$i" -lt "$((DURATION * RPS))" ]; do
    # auth: валидный и невалидный логин (невалидный = neutral, НЕ ошибка)
    ensure_user
    curl -s -o /dev/null -X POST "$GATEWAY/api/auth/login" \
      -H 'Content-Type: application/json' \
      -d "{\"email\":\"$email\",\"password\":\"wrong\"}" || true

    # feed: с токеном и без него (401 = neutral)
    if [ -n "${TOKEN:-}" ]; then
      curl -s -o /dev/null "$GATEWAY/api/feed/" -H "Authorization: $TOKEN" || true
      curl -s -o /dev/null "$GATEWAY/api/chat/conversation" -H "Authorization: $TOKEN" || true
      # Бизнес-операции: наполняют wave_feed_publications_total (peer SLI).
      id=$(curl -s -X POST "$GATEWAY/api/feed/" \
        -H "Authorization: $TOKEN" -H 'Content-Type: application/json' \
        -d "{\"text\":\"load $i\"}" 2>/dev/null | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
      if [ -n "$id" ]; then
        curl -s -o /dev/null -X POST "$GATEWAY/api/feed/$id/comment/" \
          -H "Authorization: $TOKEN" -H 'Content-Type: application/json' \
          -d '{"text":"nice"}' || true
      fi
    fi
    curl -s -o /dev/null "$GATEWAY/api/feed/" || true

    i=$((i + 1))
    sleep "$(awk "BEGIN{print 1/$RPS}")"
  done
}

scale() {
  kubectl -n "$NS" scale deployment "$1" --replicas="$2"
}

case "${1:-}" in
traffic)
  preflight
  echo "== нормальный трафик ${DURATION}s =="
  traffic
  ;;
break-feed)
  preflight
  echo "== feed-service → 0 реплик (gateway начнёт отдавать 502) =="
  scale feed-service 0
  sleep 5
  traffic
  ;;
break-db)
  preflight
  echo "== feed-service жив, но БД недоступна (5xx изнутри) =="
  # Меняем хост БД, а не пароль: сервис не упадёт на миграциях,
  # а запросы будут падать с 500 — именно это и нужно для reason=server_error.
  kubectl -n "$NS" set env deployment/feed-service DB_HOST=feed-db-broken
  kubectl -n "$NS" rollout status deployment/feed-service --timeout=120s || true
  traffic
  ;;
break-auth)
  preflight
  echo "== auth-service → 0 реплик (логины и весь /api/auth падают) =="
  scale auth-service 0
  sleep 5
  # Без auth не получить токен — шлём анонимный трафик по всем слоям.
  TOKEN=""
  traffic
  ;;
heal)
  echo "== восстановление =="
  scale auth-service 1
  scale feed-service 1
  kubectl -n "$NS" set env deployment/feed-service DB_HOST=feed-db
  kubectl -n "$NS" rollout status deployment/feed-service --timeout=180s || true
  kubectl -n "$NS" rollout status deployment/auth-service --timeout=180s || true
  ;;
urls)
  cat <<'EOF'
vmui: http://localhost:8428/vmui

Все запросы ниже работают БЕЗ vmalert — вставляй в поле запроса vmui.
Полный список с пояснениями: slo/query.txt

# SLI RBSLO (доля не-5xx)
1 - sum by (service) (rate(wave_slo_requests_total{class="bad"}[5m])) / clamp_min(sum by (service) (rate(wave_slo_requests_total[5m])), 1e-9)

# SLI TBSLO (доля хороших минут)
sum by (service) (wave_slo_window_good{window="5m"}) / clamp_min(sum by (service) (wave_slo_window_total{window="5m"}), 1)

# SLA (доля успешных за час)
1 - sum(increase(wave_slo_requests_total{class="bad"}[1h])) / clamp_min(sum(increase(wave_slo_requests_total[1h])), 1)

# SLO (остаток бюджета feed; для демо замени 30d на 10m)
1 - (1 - (1 - sum(increase(wave_slo_requests_total{service="feed",class="bad"}[30d])) / clamp_min(sum(increase(wave_slo_requests_total{service="feed"}[30d])), 1))) / 0.005

# Причины 5xx (bad_gateway vs server_error)
sum by (service, reason) (rate(wave_slo_requests_total{class="bad"}[5m]))

# Живость
max by (service) (wave_slo_probe_up)
EOF
  ;;
panels)
  # Быстрый smoke-тест: заполняется ли база метрик.
  VM="${VM:-http://127.0.0.1:8428}"
  echo "== проверка метрик в $VM =="
  if ! curl -s -o /dev/null --max-time 3 "$VM/api/v1/query?query=1" 2>/dev/null; then
    cat >&2 <<'EOF'
!! VictoriaMetrics недоступна.

  kubectl -n monitoring port-forward svc/vm-single-victoria-metrics-single-server 8428:8428 &
EOF
    exit 1
  fi
  for q in \
    'wave_slo_requests_total' \
    'wave_slo_window_total' \
    'wave_slo_probe_up' \
    'wave_auth_logins_total' \
    'wave_feed_publications_total' \
    'wave_chat_messages_total' \
    'wave_kafka_events_processed_total'; do
    n=$(curl -s --get --data-urlencode "query=count($q)" "$VM/api/v1/query" \
         | sed -n 's/.*"value":\[.*,"\([^"]*\)"\].*/\1/p')
    printf '  %-38s %s\n' "$q" "${n:-нет данных}"
  done
  ;;
*)
  sed -n '2,20p' "$0"
  exit 1
  ;;
esac
