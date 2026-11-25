INSERT INTO cinema.daily_metrics_stage (day, metric, movie_id, value, computed_at)
WITH
    {day:Date} AS report_day,
    day_events AS (
        SELECT DISTINCT event_id, user_id, movie_id, event_type, progress_seconds
        FROM cinema.movie_events
        WHERE toDate(timestamp) = report_day
    ),
    totals AS (
        SELECT count() AS events,
               uniqExact(user_id) AS users,
               avgOrNullIf(progress_seconds, event_type = 'VIEW_FINISHED') AS avg_seconds,
               countIf(event_type = 'VIEW_FINISHED') /
                   nullIf(countIf(event_type = 'VIEW_STARTED'), 0) AS conversion
        FROM day_events
    ),
    top_movies AS (
        SELECT movie_id, count() AS views
        FROM day_events
        WHERE event_type = 'VIEW_STARTED'
        GROUP BY movie_id
        ORDER BY views DESC, movie_id
        LIMIT 10
    ),
    first_views AS (
        SELECT user_id,
               minIf(toDate(timestamp), event_type = 'VIEW_STARTED') AS first_day,
               countIf(toDate(timestamp) = report_day) > 0 AS returned
        FROM cinema.movie_events
        WHERE toDate(timestamp) <= report_day
        GROUP BY user_id
        HAVING countIf(event_type = 'VIEW_STARTED') > 0
    ),
    retention AS (
        SELECT countIf(first_day = report_day - 1 AND returned) /
                   nullIf(countIf(first_day = report_day - 1), 0) AS d1,
               countIf(first_day = report_day - 7 AND returned) /
                   nullIf(countIf(first_day = report_day - 7), 0) AS d7
        FROM first_views
    )
SELECT report_day, item.1, '', item.2, now64(3, 'UTC')
FROM totals
ARRAY JOIN [('events_processed', toFloat64(events)), ('dau', toFloat64(users)),
            ('avg_watch_seconds', avg_seconds), ('conversion', conversion)] AS item
UNION ALL
SELECT report_day, 'movie_views', movie_id, toFloat64(views), now64(3, 'UTC')
FROM top_movies
UNION ALL
SELECT report_day, item.1, '', item.2, now64(3, 'UTC')
FROM retention
ARRAY JOIN [('retention_d1', d1), ('retention_d7', d7)] AS item
UNION ALL
SELECT report_day, concat('cohort_day_', toString(age)), '', avg(returned), now64(3, 'UTC')
FROM (
    SELECT dateDiff('day', first_day, report_day) AS age, returned
    FROM first_views
)
WHERE age BETWEEN 0 AND 7
GROUP BY age
