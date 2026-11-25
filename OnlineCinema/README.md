# Online Cinema

Сервис аналитики онлайн-кинотеатра на Go. HTTP API принимает события просмотра и публикует их в Kafka. Ключ `user_id` сохраняет порядок событий одного зрителя в пределах партиции. ClickHouse хранит события и дневные метрики для Grafana. Из PostgreSQL дневные отчёты выгружаются в S3 в формате CSV.

## Запуск

Запуск из корня репозитория. Все последующие команды выполняются в каталоге `OnlineCinema` в одной сессии терминала.

```sh
cd OnlineCinema
docker compose up -d --build --wait
docker compose ps
```

## Пример использования

### 1. Публикация события

При успешной записи `POST /events` возвращает `201 Created` и идентификатор `event_id`. Дата в UTC и идентификатор сессии используются в следующих запросах.

```sh
NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
DAY=${NOW%T*}
SESSION=demo-$(date +%s)
curl -fsS http://localhost:8080/events -H 'Content-Type: application/json' -d @- <<JSON
{"user_id":"viewer-1","movie_id":"film-1","event_type":"VIEW_STARTED",
 "timestamp":"$NOW","device_type":"TV","session_id":"$SESSION","progress_seconds":0}
JSON
```

### 2. Проверка доставки в ClickHouse

Событие обычно появляется в таблице через несколько секунд. Строка `VIEW_STARTED` с указанным идентификатором сессии подтверждает доставку.

```sh
curl -fsS -u cinema:cinema http://localhost:8123/ --data-binary \
  "SELECT timestamp, user_id, movie_id, event_type FROM cinema.movie_events WHERE session_id='$SESSION' FORMAT PrettyCompact"
```

### 3. Дневные метрики и Grafana

По умолчанию сервис каждые пять минут пересчитывает показатели за текущий и предыдущий день по UTC. Запрос ниже запускает пересчёт за дату события, а SQL-запрос выводит результат из PostgreSQL.

```sh
curl -fsS -X POST "http://localhost:8082/recalculate?date=$DAY"
docker compose exec postgres psql -U cinema -d cinema -c \
  "SELECT metric, movie_id, value FROM daily_metrics WHERE day='$DAY' ORDER BY metric, movie_id"
```

Дашборд показывает число активных зрителей за день (DAU), конверсию просмотров и долю зрителей, вернувшихся в каждый из семи дней после первого просмотра. Локальный вход — `admin` / `cinema-local`. [Дашборд Grafana](http://localhost:3000/d/online-cinema)

### 4. Экспорт в S3

Отчёты за завершённые дни выгружаются автоматически. Ручной экспорт за выбранную дату создаёт или обновляет CSV по тому же пути.

```sh
curl -fsS -X POST "http://localhost:8082/export?date=$DAY"
curl -fsS --aws-sigv4 aws:amz:us-east-1:s3 -u cinema-local:cinema-local-secret \
  "http://localhost:8333/movie-analytics/daily/$DAY/aggregates.csv"
```

### 5. Схема событий и репликация

Schema Registry хранит Avro-схему события. Топик `movie-events` состоит из трёх партиций с двумя репликами у каждой.

```sh
curl -fsS http://localhost:8081/subjects/movie-events-value/versions/1/schema
docker compose exec kafka kafka-topics --bootstrap-server kafka:29092 --describe --topic movie-events
```

### Генератор событий

Встроенный генератор имитирует действия зрителей. В примере интервал составляет две секунды. Данные на дашборде обновляются после очередного или ручного пересчёта.

```sh
GENERATE_EVENTS=true GENERATOR_INTERVAL=2s docker compose up -d --no-deps producer
docker compose logs --tail=10 producer
```

## Интеграционные тесты

```sh
# Отправка через HTTP → Kafka → проверка всех полей в ClickHouse
docker compose run --rm --build --no-deps pipeline-test

# Дневные метрики в обеих базах, повторный пересчёт и экспорт в S3
sh integration/aggregation_test.sh
```

Проверка отказоустойчивости выполняется на работающем кластере. При остановленном брокере тест доставки должен завершиться с результатом `PASS`. Последняя команда запускает брокер снова.

```sh
docker compose stop kafka
docker compose run --rm --no-deps pipeline-test
docker compose start kafka
```

## Остановка

Команда `docker compose down` останавливает сервисы и сохраняет данные в томах Docker.
