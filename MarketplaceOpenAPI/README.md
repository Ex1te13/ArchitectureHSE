# Marketplace API

Сервис маркетплейса на Go: CRUD товаров по OpenAPI-контракту (contract-first), PostgreSQL, миграции Flyway, валидация запросов по контракту, единый формат ошибок и JSON-логирование.

## Запуск

```bash
docker compose up --build
```

Поднимутся три контейнера по очереди: `postgres` → `flyway` (применяет миграции и завершается) → `app` (сервер на `http://localhost:8080`).

Остановить и удалить данные:

```bash
docker compose down -v
```

## Структура

```
api/openapi.yaml          контракт API
api/oapi-codegen.yaml     настройки генератора
internal/api/             сгенерированный код
internal/handler/         обработчики запросов
internal/repository/      SQL-запросы к PostgreSQL
internal/middleware/      логирование, валидация, формат ошибок
cmd/server/main.go        точка входа: сборка всех частей
migrations/               SQL-миграции Flyway
```

Запрос проходит так: `Logging → Validator → роутер → strict-обёртка → handler → repository → PostgreSQL`.


## API

| Метод | Путь | Описание |
|---|---|---|
| `POST` | `/products` | Создать товар |
| `GET` | `/products/{id}` | Получить товар |
| `GET` | `/products?page=0&size=20&status=ACTIVE&category=phones` | Список с пагинацией и фильтрами |
| `PUT` | `/products/{id}` | Обновить товар (полная замена) |
| `DELETE` | `/products/{id}` | Мягкое удаление: статус `ARCHIVED` |

Без фильтра `status` архивные товары в списке не показываются.

Ошибки возвращаются в одном формате:

```json
{"error_code": "VALIDATION_ERROR", "message": "request validation failed",
 "details": {"fields": [{"field": "price", "message": "number must be at least 0.01"}]}}
```

## Примеры

```bash
curl -i -X POST localhost:8080/products -H "Content-Type: application/json" \
  -d '{"name":"Ноутбук","price":129990.50,"stock":3,"category":"laptops","status":"ACTIVE"}'
```

```bash
curl -i "localhost:8080/products?page=0&size=10"
```

Посмотреть таблицу в базе:

```bash
docker compose exec postgres psql -U admin -d marketplace -c "SELECT * FROM products;"
```

## Логи

Каждый запрос пишется одной JSON-строкой с полями `request_id`, `method`, `endpoint`, `status_code`, `duration_ms`, `user_id`, `timestamp`. Для `POST`/`PUT`/`DELETE` логируется тело запроса с замаскированными паролями и токенами. `request_id` возвращается клиенту в заголовке `X-Request-Id`.

```bash
docker compose logs -f app
```
