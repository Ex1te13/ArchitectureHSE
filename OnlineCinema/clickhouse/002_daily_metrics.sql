CREATE TABLE IF NOT EXISTS cinema.daily_metrics
(
    day Date,
    metric String,
    movie_id String,
    value Nullable(Float64),
    computed_at DateTime64(3, 'UTC')
)
ENGINE = MergeTree
PARTITION BY day
ORDER BY (metric, movie_id);

CREATE TABLE IF NOT EXISTS cinema.daily_metrics_stage AS cinema.daily_metrics;
