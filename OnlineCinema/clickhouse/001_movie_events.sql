CREATE DATABASE IF NOT EXISTS cinema;

CREATE TABLE IF NOT EXISTS cinema.movie_events
(
    event_id UUID,
    user_id String,
    movie_id String,
    event_type Enum8(
        'VIEW_STARTED' = 1, 'VIEW_FINISHED' = 2, 'VIEW_PAUSED' = 3,
        'VIEW_RESUMED' = 4, 'LIKED' = 5, 'SEARCHED' = 6
    ),
    timestamp DateTime64(3, 'UTC'),
    device_type Enum8('MOBILE' = 1, 'DESKTOP' = 2, 'TV' = 3, 'TABLET' = 4),
    session_id String,
    progress_seconds Nullable(Int32)
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(timestamp)
ORDER BY (toDate(timestamp), event_type, user_id, timestamp, event_id);

CREATE TABLE IF NOT EXISTS cinema.movie_events_kafka AS cinema.movie_events
ENGINE = Kafka
SETTINGS kafka_broker_list = 'kafka:29092,kafka-2:29092,kafka-3:29092',
         kafka_topic_list = 'movie-events',
         kafka_group_name = 'clickhouse-movie-events',
         kafka_format = 'AvroConfluent',
         format_avro_schema_registry_url = 'http://schema-registry:8081',
         kafka_flush_interval_ms = 1000;

CREATE MATERIALIZED VIEW IF NOT EXISTS cinema.movie_events_ingest
TO cinema.movie_events AS
SELECT event_id, user_id, movie_id, event_type, timestamp, device_type, session_id, progress_seconds
FROM cinema.movie_events_kafka;
