CREATE TYPE booking_status AS ENUM ('CONFIRMED', 'CANCELLED');

CREATE TABLE bookings (
    id              UUID           PRIMARY KEY,
    user_id         TEXT           NOT NULL CHECK (btrim(user_id) <> ''),
    flight_id       UUID           NOT NULL,
    passenger_name  TEXT           NOT NULL CHECK (btrim(passenger_name) <> ''),
    passenger_email TEXT           NOT NULL CHECK (passenger_email ~ '^[^@\s]+@[^@\s]+\.[^@\s]+$'),
    seat_count      INT            NOT NULL CHECK (seat_count > 0),
    total_price     BIGINT         NOT NULL CHECK (total_price > 0),
    currency        CHAR(3)        NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    status          booking_status NOT NULL DEFAULT 'CONFIRMED',
    created_at      TIMESTAMPTZ    NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ    NOT NULL DEFAULT now()
);

CREATE INDEX bookings_user_idx ON bookings (user_id, created_at DESC);
