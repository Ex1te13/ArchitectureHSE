set -eu
cd "$(dirname "$0")/.."
tmp=$(mktemp -d)
project="cinema-metrics-test-$(date +%s)-$$"
cat > "$tmp/compose.yml" <<'YAML'
services:
  clickhouse:
    ports: !reset []
  s3:
    ports: !override ["127.0.0.1::8333"]
  aggregation:
    ports: !override ["127.0.0.1::8082"]
YAML
compose() { docker compose -p "$project" -f docker-compose.yml -f "$tmp/compose.yml" "$@"; }
cleanup() { compose down -v --rmi local; rm -f "$tmp/compose.yml" "$tmp/report.csv"; rmdir "$tmp"; }
trap cleanup EXIT
compose up -d --build --wait aggregation s3
endpoint="http://$(compose port aggregation 8082)/recalculate"
ch() { compose exec -T clickhouse clickhouse-client --user cinema --password cinema "$@"; }
pg() { compose exec -T postgres psql -XAt -F ' ' -U cinema -d cinema -c "$1"; }

ch --query "INSERT INTO cinema.movie_events
  SELECT generateUUIDv4(), u, m, t, dt, 'TV', u, progress
  FROM values('u String, m String, t String, dt DateTime, progress Nullable(Int32)',
    ('a','X','VIEW_STARTED','2025-01-09 12:00:00',0),
    ('b','X','VIEW_STARTED','2025-01-09 12:00:00',0),
    ('c','Y','VIEW_STARTED','2025-01-03 12:00:00',0),
    ('d','Y','VIEW_STARTED','2025-01-03 12:00:00',0),
    ('a','X','VIEW_STARTED','2025-01-10 12:00:00',0),
    ('a','X','VIEW_FINISHED','2025-01-10 12:02:00',120),
    ('c','Y','VIEW_STARTED','2025-01-10 13:00:00',0),
    ('c','Y','VIEW_FINISHED','2025-01-10 13:04:00',240),
    ('c','Y','VIEW_STARTED','2025-01-10 14:00:00',0),
    ('e','X','LIKED','2025-01-10 15:00:00',NULL))"

expected='avg_watch_seconds - 180
cohort_day_1 - 0.5
cohort_day_7 - 0.5
conversion - 0.6667
dau - 3
events_processed - 6
movie_views X 1
movie_views Y 2
retention_d1 - 0.5
retention_d7 - 0.5'
for run in 1 2; do
    curl -fsS --retry-connrefused --retry 3 -X POST "$endpoint?date=2025-01-10"
    actual_pg=$(pg "SELECT metric, coalesce(nullif(movie_id,''),'-'), round(value::numeric,4)::float8 FROM daily_metrics WHERE day='2025-01-10' ORDER BY metric,movie_id")
    actual_ch=$(ch --query "SELECT metric, if(empty(movie_id),'-',movie_id), round(value,4) FROM cinema.daily_metrics WHERE day='2025-01-10' ORDER BY metric,movie_id" | tr '\t' ' ')
    if [ "$actual_pg" != "$expected" ] || [ "$actual_ch" != "$expected" ]; then
        printf 'Expected:\n%s\nPostgreSQL:\n%s\nClickHouse:\n%s\n' "$expected" "$actual_pg" "$actual_ch"
        exit 1
    fi
done
echo 'Daily metrics and repeated recalculation passed.'

object="http://$(compose port s3 8333)/movie-analytics/daily/2025-01-10/aggregates.csv"
for run in 1 2; do
    curl -fsS -X POST "http://$(compose port aggregation 8082)/export?date=2025-01-10"
    curl -fsS --aws-sigv4 aws:amz:us-east-1:s3 -u cinema-local:cinema-local-secret "$object" > "$tmp/report.csv"
    test "$(wc -l < "$tmp/report.csv" | tr -d ' ')" = 11
    grep -q '^2025-01-10,dau,,3,' "$tmp/report.csv"
    grep -q '^2025-01-10,retention_d7,,0.5,' "$tmp/report.csv"
done
echo 'CSV export passed.'
