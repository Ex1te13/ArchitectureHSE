#!/bin/sh
set -eu
ch() { clickhouse-client --host clickhouse --user cinema --password cinema "$@"; }

# Kafka Engine stores no rows. Recreate only the reader when upgrading the old bootstrap address.
old_reader=$(ch --query "SELECT count() FROM system.tables WHERE database='cinema' AND name='movie_events_kafka' AND position(create_table_query, 'kafka-2:29092')=0")
if [ "$old_reader" = 1 ]; then
  ch --multiquery --query "DROP VIEW IF EXISTS cinema.movie_events_ingest SYNC; DROP TABLE cinema.movie_events_kafka SYNC"
fi
ch --queries-file /migrations/001_movie_events.sql /migrations/002_daily_metrics.sql
