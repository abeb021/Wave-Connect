for svc in auth profile feed chat; do
  echo "=== Testing $svc ==="
  docker compose exec $svc go test ./... -v
done