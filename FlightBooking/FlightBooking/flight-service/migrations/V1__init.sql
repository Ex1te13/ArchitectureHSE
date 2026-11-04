CREATE TYPE flight_status AS ENUM ('SCHEDULED', 'DEPARTED', 'CANCELLED', 'COMPLETED');
CREATE TYPE reservation_status AS ENUM ('ACTIVE', 'RELEASED', 'EXPIRED');

CREATE TABLE airlines (
    code CHAR(2) PRIMARY KEY CHECK (code ~ '^[A-Z0-9]{2}$'),
    name TEXT    NOT NULL CHECK (btrim(name) <> '')
);

CREATE TABLE airports (
    iata_code CHAR(3) PRIMARY KEY CHECK (iata_code ~ '^[A-Z]{3}$'),
    name      TEXT    NOT NULL CHECK (btrim(name) <> ''),
    city      TEXT    NOT NULL CHECK (btrim(city) <> '')
);

CREATE TABLE flights (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    airline_code    CHAR(2)       NOT NULL REFERENCES airlines (code),
    flight_number   VARCHAR(8)    NOT NULL CHECK (flight_number ~ '^[0-9]{1,4}[A-Z]?$'),
    origin          CHAR(3)       NOT NULL REFERENCES airports (iata_code),
    destination     CHAR(3)       NOT NULL REFERENCES airports (iata_code),
    departure_time  TIMESTAMPTZ   NOT NULL,
    arrival_time    TIMESTAMPTZ   NOT NULL,
    total_seats     INT           NOT NULL CHECK (total_seats > 0),
    available_seats INT           NOT NULL,
    price_amount    BIGINT        NOT NULL CHECK (price_amount > 0),
    currency        CHAR(3)       NOT NULL DEFAULT 'RUB' CHECK (currency ~ '^[A-Z]{3}$'),
    status          flight_status NOT NULL DEFAULT 'SCHEDULED',
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),

    CONSTRAINT flights_available_seats_check CHECK (available_seats BETWEEN 0 AND total_seats),
    CONSTRAINT flights_times_check CHECK (arrival_time > departure_time),
    CONSTRAINT flights_route_check CHECK (origin <> destination)
);

CREATE UNIQUE INDEX flights_number_date_uq
    ON flights (airline_code, flight_number, ((departure_time AT TIME ZONE 'UTC')::date));

CREATE INDEX flights_search_idx ON flights (origin, destination, departure_time);

CREATE TABLE seat_reservations (
    id         UUID               PRIMARY KEY DEFAULT gen_random_uuid(),
    flight_id  UUID               NOT NULL REFERENCES flights (id) ON DELETE RESTRICT,
    booking_id UUID               NOT NULL UNIQUE,
    seat_count INT                NOT NULL CHECK (seat_count > 0),
    status     reservation_status NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ        NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ        NOT NULL DEFAULT now()
);

CREATE INDEX seat_reservations_flight_idx ON seat_reservations (flight_id);
