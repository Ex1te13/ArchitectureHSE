set -e

MASTER_NAME="${REDIS_MASTER_NAME:-mymaster}"

cat > /tmp/sentinel.conf <<EOF
port 26379
sentinel resolve-hostnames yes
sentinel announce-hostnames yes
sentinel announce-ip ${SENTINEL_ANNOUNCE_HOST}
sentinel monitor ${MASTER_NAME} ${REDIS_MASTER_HOST:-redis-master} 6379 ${SENTINEL_QUORUM:-2}
sentinel down-after-milliseconds ${MASTER_NAME} 5000
sentinel failover-timeout ${MASTER_NAME} 10000
sentinel parallel-syncs ${MASTER_NAME} 1
EOF

exec redis-sentinel /tmp/sentinel.conf
