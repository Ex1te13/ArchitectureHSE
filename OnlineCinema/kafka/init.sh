set -euo pipefail
brokers=kafka:29092,kafka-2:29092,kafka-3:29092
kafka-topics --bootstrap-server "$brokers" --create --if-not-exists --topic movie-events \
  --partitions 3 --replication-factor 2 --config min.insync.replicas=1
kafka-configs --bootstrap-server "$brokers" --entity-type topics --entity-name movie-events \
  --alter --add-config min.insync.replicas=1

topics=$(kafka-topics --bootstrap-server "$brokers" --list)
for topic in movie-events __consumer_offsets _schemas; do
  if ! grep -qx "$topic" <<< "$topics"; then continue; fi
  info=$(kafka-topics --bootstrap-server "$brokers" --describe --topic "$topic")
  plan=$(awk -v topic="$topic" '
    /Partition:/ {
      for (i=1; i<=NF; i++) {
        if ($i=="Partition:") partition=$(i+1)
        if ($i=="Replicas:") replicas=$(i+1)
      }
      if (split(replicas, r, ",") == 1) {
        printf "%s{\"topic\":\"%s\",\"partition\":%d,\"replicas\":[%d,%d]}", sep,topic,partition,r[1],r[1]%3+1
        sep=","
      }
    }' <<< "$info")
  if [[ -n "$plan" ]]; then
    printf '{"version":1,"partitions":[%s]}\n' "$plan" > /tmp/reassign.json
    kafka-reassign-partitions --bootstrap-server "$brokers" --reassignment-json-file /tmp/reassign.json --execute
    for ((i=0; i<60; i++)); do
      status=$(kafka-reassign-partitions --bootstrap-server "$brokers" --reassignment-json-file /tmp/reassign.json --verify)
      if [[ "$status" != *"in progress"* && "$status" != *"still in"* ]]; then break; fi
      sleep 1
    done
  fi
  kafka-topics --bootstrap-server "$brokers" --describe --topic "$topic" | awk '
    /Partition:/ {for (i=1;i<=NF;i++) if ($i=="Replicas:" && $(i+1) !~ /,/) exit 1}'
done
