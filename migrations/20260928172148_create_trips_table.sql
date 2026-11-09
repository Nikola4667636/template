-- +goose Up
-- +goose StatementBegin
CREATE TABLE trips (
    id              UUID PRIMARY KEY,
    user_id         UUID NOT NULL,
    driver_id       UUID NOT NULL,

    start_latitude  DOUBLE PRECISION NOT NULL CHECK (start_latitude BETWEEN -90 AND 90),
    start_longitude DOUBLE PRECISION NOT NULL CHECK (start_longitude BETWEEN -180 AND 180),
    end_latitude    DOUBLE PRECISION NOT NULL CHECK (end_latitude BETWEEN -90 AND 90),
    end_longitude   DOUBLE PRECISION NOT NULL CHECK (end_longitude BETWEEN -180 AND 180),

    price           BIGINT NOT NULL CHECK (price >= 0),
    status          TEXT NOT NULL CHECK (status IN ('active', 'completed')),

    started_at      TIMESTAMPTZ NOT NULL,
    finished_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CHECK (
        (status = 'active'    AND finished_at IS NULL) OR
        (status = 'completed' AND finished_at IS NOT NULL)
    )
);

CREATE INDEX trips_status_started_at_idx ON trips (status, started_at);
CREATE UNIQUE INDEX idx_trips_driver_active ON trips (driver_id) WHERE status = 'active';

CREATE TABLE trip_status_history (
    id          BIGSERIAL PRIMARY KEY,
    trip_id     UUID NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    from_status TEXT,
    to_status   TEXT NOT NULL CHECK (to_status IN ('active', 'completed')),
    reason      TEXT,
    changed_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX trip_status_history_trip_changed_idx
    ON trip_status_history (trip_id, changed_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS trip_status_history;
DROP TABLE IF EXISTS trips;
-- +goose StatementEnd