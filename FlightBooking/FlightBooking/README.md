# Flight Booking

Домашнее задание №3: бронирование авиабилетов. Два сервиса на Go, которые общаются по gRPC, у каждого своя база PostgreSQL, а Flight Service кеширует данные в Redis.

```
Client (REST) → Booking Service → (gRPC) → Flight Service
                      ↓                          ↓
                 PostgreSQL               PostgreSQL + Redis
```

- **Booking Service** — REST API для пользователя: поиск рейсов, бронирования.
- **Flight Service** — рейсы и места, наружу только gRPC.

## Как запустить

```bash
docker compose up --build
```

Миграции применяются сами при старте (Flyway). В базе сразу есть тестовые рейсы.

| Что | Где |
|---|---|
| Booking Service (REST) | `http://localhost:8080` |
| Flight Service (gRPC) | `localhost:50051` |
| база Flight Service | `localhost:5433`, логин/пароль `flight`, база `flights` |
| база Booking Service | `localhost:5434`, логин/пароль `booking`, база `bookings` |

Остановить и удалить данные:

```bash
docker compose down -v
```

## Что где лежит

```
proto/flight/v1/flight.proto    gRPC-контракт Flight Service
gen/                            код, сгенерированный из .proto (make proto)
docs/                           ER-диаграмма
flight-service/
  migrations/                   таблицы и тестовые данные
  internal/repository/          SQL и транзакции
  internal/cache/               кеш в Redis
  internal/server/              gRPC-методы, проверка API-ключа, логи
booking-service/
  migrations/                   таблица бронирований
  internal/repository/          SQL
  internal/handler/             REST-эндпоинты
  internal/flightclient/        API-ключ, retry и circuit breaker для вызовов Flight Service
redis/sentinel.sh               конфиг Redis Sentinel
docker-compose.yml
```

## REST API

| Запрос | Что делает |
|---|---|
| `GET /flights?origin=SVO&destination=LED&date=2026-12-01` | поиск рейсов (`date` можно не передавать) |
| `GET /flights/{id}` | рейс по ID |
| `POST /bookings` | создать бронирование |
| `GET /bookings/{id}` | бронирование по ID |
| `POST /bookings/{id}/cancel` | отменить бронирование |
| `GET /bookings?user_id=u1` | бронирования пользователя |

Цены везде в копейках: `{"amount": 650000, "currency": "RUB"}` — это 6500 ₽.

Тестовые рейсы: SVO → LED на 1 и 2 декабря 2026, рейс `…0006` на 2 места, рейс `…0008` на 1 место и отменённый рейс `…0007`.

```bash
curl "localhost:8080/flights?origin=SVO&destination=LED"
```

```bash
curl -X POST localhost:8080/bookings -H "Content-Type: application/json" \
  -d '{"user_id":"u1","flight_id":"a1000000-0000-4000-8000-000000000001","passenger_name":"Иван Петров","passenger_email":"ivan@example.com","seat_count":2}'
```

```bash
curl "localhost:8080/bookings?user_id=u1"
```

```bash
curl -X POST localhost:8080/bookings/<id>/cancel
```

Ошибки приходят в одном виде: `{"error_code": "...", "message": "..."}`.

| HTTP | Когда |
|---|---|
| 400 `VALIDATION_ERROR` | неправильные данные в запросе |
| 404 `FLIGHT_NOT_FOUND`, `BOOKING_NOT_FOUND` | рейса или бронирования нет |
| 409 `NOT_ENOUGH_SEATS` | не хватает мест |
| 409 `FLIGHT_NOT_AVAILABLE` | рейс отменён или уже улетел |
| 409 `BOOKING_NOT_CANCELLABLE` | бронирование уже отменено |
| 503 `FLIGHT_SERVICE_UNAVAILABLE` | Flight Service недоступен |
| 503 `FLIGHT_SERVICE_CIRCUIT_OPEN` | circuit breaker открыт (п.10) |
| 504 `FLIGHT_SERVICE_TIMEOUT` | Flight Service не ответил вовремя |

## Как сделан каждый пункт

### 1. gRPC-контракт

Контракт в [proto/flight/v1/flight.proto](proto/flight/v1/flight.proto). Методы: `SearchFlights`, `GetFlight`, `ReserveSeats`, `ReleaseReservation` и ещё `UpdateFlight` (нужен для п.7).

- Методы — это действия («зарезервировать места»), а не «обновить таблицу». `ReserveSeats` сам проверяет места и резервирует их за один раз, поэтому клиенту не нужно делать это в два шага.
- Даты — `google.protobuf.Timestamp`, статусы — `enum`.
- Деньги — целое число в копейках, чтобы не было ошибок округления.
- Код генерируется через `protoc` командой `make proto`.

Какие ошибки возвращают методы:

| Метод | Ошибки |
|---|---|
| все | `UNAUTHENTICATED` — нет API-ключа или он неверный |
| `SearchFlights` | `INVALID_ARGUMENT` — неправильный код аэропорта |
| `GetFlight` | `INVALID_ARGUMENT` — id не UUID; `NOT_FOUND` — рейса нет |
| `ReserveSeats` | `INVALID_ARGUMENT`; `NOT_FOUND` — рейса нет; `FAILED_PRECONDITION` — рейс не SCHEDULED; `RESOURCE_EXHAUSTED` — мало мест; `ALREADY_EXISTS` — на этот `booking_id` уже есть другая резервация |
| `ReleaseReservation` | `INVALID_ARGUMENT`; `NOT_FOUND` — резервации нет; `FAILED_PRECONDITION` — резервация истекла |
| `UpdateFlight` | `INVALID_ARGUMENT` — например, прилёт раньше вылета; `NOT_FOUND` — рейса нет |

### 2. ER-диаграмма

Диаграмма в [docs/er.md](docs/er.md) (исходник в draw.io — `docs/er.drawio`).

- Авиакомпании и аэропорты вынесены в отдельные таблицы, чтобы их названия не повторялись в каждом рейсе (3NF).
- Ограничения прямо в базе: мест не может быть меньше 0 или больше общего числа, цена больше 0, прилёт позже вылета, вылет и прилёт в разных аэропортах.
- Один рейс (номер + дата) может быть только один раз — уникальный индекс.
- `booking_id` в резервациях уникальный: одно бронирование — одна резервация.

### 3. Базы и сервисы

- Две отдельные PostgreSQL: `flight-db` и `booking-db`.
- Миграции через Flyway, запускаются сами перед сервисами.
- Flight Service — gRPC-сервер, Booking Service — REST API из таблицы выше.

### 4. Вызовы по gRPC

При создании бронирования Booking Service:

1. вызывает `GetFlight` — узнаёт цену (нет рейса → 404);
2. вызывает `ReserveSeats` — резервирует места (мало мест → 409, бронирование не создаётся);
3. считает цену `seat_count × цена рейса` и сохраняет бронирование со статусом `CONFIRMED`.

При отмене вызывает `ReleaseReservation`, места возвращаются.

Посмотреть порядок вызовов:

```bash
docker compose logs -f flight-service
```

### 5. Транзакции

В Flight Service:

- `ReserveSeats` — в одной транзакции: `SELECT … FOR UPDATE` блокирует строку рейса, проверяются места, уменьшается `available_seats`, создаётся резервация.
- `ReleaseReservation` — в одной транзакции: места возвращаются и резервация становится `RELEASED`.

Из-за `FOR UPDATE` два запроса не могут купить последнее место одновременно: второй ждёт, пока первый закончит, и видит, что мест уже нет.

Проверка — 10 одновременных бронирований рейса на 1 место, успешное должно быть только одно:

```bash
for i in $(seq 1 10); do curl -s -o /dev/null -w '%{http_code}\n' -X POST localhost:8080/bookings -H "Content-Type: application/json" -d "{\"user_id\":\"race$i\",\"flight_id\":\"a1000000-0000-4000-8000-000000000008\",\"passenger_name\":\"P$i\",\"passenger_email\":\"p$i@example.com\",\"seat_count\":1}" & done | sort | uniq -c
```

Ответ: один `201`, девять `409`.

В Booking Service: если `ReserveSeats` не прошёл, бронирование не создаётся. Если места зарезервировались, а сохранить бронирование в базу не получилось, Booking Service вызывает `ReleaseReservation`, чтобы места не остались занятыми.

### 6. Аутентификация

Выбран API Key. Booking Service добавляет к каждому gRPC-вызову заголовок `x-api-key`, Flight Service проверяет его у всех методов. Нет ключа или он неверный — `UNAUTHENTICATED`.

Ключ задаётся переменной окружения `FLIGHT_API_KEY` (в `docker-compose.yml` по умолчанию `dev-secret-key`).

Проверка (нужен `grpcurl`, ставится через `brew install grpcurl`):

```bash
grpcurl -plaintext -d '{"flight_id":"a1000000-0000-4000-8000-000000000001"}' localhost:50051 flight.v1.FlightService/GetFlight
```

```bash
grpcurl -plaintext -H 'x-api-key: dev-secret-key' -d '{"flight_id":"a1000000-0000-4000-8000-000000000001"}' localhost:50051 flight.v1.FlightService/GetFlight
```

Первая команда вернёт `Unauthenticated`, вторая — рейс.

### 7. Кеш в Redis

Flight Service сначала ищет данные в Redis. Если нашёл — отвечает сразу (`cache hit`). Если нет — берёт из базы и кладёт в Redis на 5 минут (`cache miss`).

| Что | Ключ | TTL |
|---|---|---|
| рейс | `flight:{id}` | 5 минут |
| поиск | `search:{origin}:{destination}:{date}` (без даты — `any`) | 5 минут |

После `ReserveSeats`, `ReleaseReservation` и `UpdateFlight` ключи этого рейса удаляются, чтобы не показывать старое число мест.

Если Redis недоступен, сервис просто работает через базу.

```bash
docker compose logs -f flight-service | grep cache
```

### 8. Retry

Если вызов Flight Service упал с `UNAVAILABLE` или `DEADLINE_EXCEEDED`, Booking Service пробует ещё раз: всего до 3 попыток, паузы 100 мс, потом 200 мс. Ошибки вроде `NOT_FOUND` или `RESOURCE_EXHAUSTED` не повторяются.

Повтор `ReserveSeats` не создаёт вторую резервацию: `booking_id` при повторе тот же, и Flight Service возвращает уже существующую резервацию.

Настройки: `RETRY_MAX_ATTEMPTS`, `RETRY_INITIAL_BACKOFF`, `RETRY_ATTEMPT_TIMEOUT`.

Проверка — «заморозить» Flight Service на полторы секунды во время запроса:

```bash
docker compose pause flight-service; (sleep 1.5; docker compose unpause flight-service) &
curl localhost:8080/flights/a1000000-0000-4000-8000-000000000001
```

```bash
docker compose logs booking-service | grep "flight service call"
```

### 9. Redis Sentinel

Redis запущен как Sentinel: `redis-master`, `redis-replica` и три sentinel'а. Если мастер падает, sentinel'ы (нужно согласие двух из трёх) делают реплику новым мастером. Flight Service подключается через sentinel'ы и сам переключается на новый мастер.

Проверка:

```bash
docker compose stop redis-master
```

Подождать секунд 10 и спросить, кто теперь мастер (должен быть `redis-replica`):

```bash
docker compose exec redis-sentinel-1 redis-cli -p 26379 sentinel get-master-addr-by-name mymaster
```

```bash
docker compose start redis-master
```

### 10. Circuit Breaker

Сделан как interceptor в Booking Service. Три состояния:

- **CLOSED** — всё работает, ошибки считаются.
- **OPEN** — после 5 ошибок Flight Service (за 30 секунд) запросы туда не отправляются, клиент сразу получает 503.
- **HALF_OPEN** — через 15 секунд пропускается один пробный запрос: получилось → CLOSED, нет → снова OPEN.

Ошибками считаются только `UNAVAILABLE` и `DEADLINE_EXCEEDED`. Настройки: `CB_FAILURE_THRESHOLD`, `CB_WINDOW`, `CB_OPEN_TIMEOUT`. Переходы пишутся в лог.

Проверка — остановить Flight Service и сделать 7 запросов: первые 5 идут около 3 секунд и возвращают 504, остальные сразу 503:

```bash
docker compose stop flight-service
```

```bash
for i in $(seq 1 7); do curl -s -o /dev/null -w '%{http_code} %{time_total}s\n' localhost:8080/flights/a1000000-0000-4000-8000-000000000001; done
```

```bash
docker compose start flight-service
```

```bash
docker compose logs booking-service | grep "circuit breaker"
```
