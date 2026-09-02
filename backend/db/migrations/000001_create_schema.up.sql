CREATE SCHEMA IF NOT EXISTS f1;

-- Seeded, repo-owned identity (ADR-0003). ids are literal constants in 000002, never generated,
-- so the same Circuit is the same id in local, CI and the deployed host.
CREATE TABLE f1.circuits (
    id           uuid PRIMARY KEY,
    circuit_key  integer     NOT NULL UNIQUE,
    short_name   text        NOT NULL,
    location     text        NOT NULL,
    country_name text        NOT NULL,
    latitude     double precision NOT NULL,
    longitude    double precision NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT NOW(),
    updated_at   timestamptz NOT NULL DEFAULT NOW()
);

-- full_name is the upstream display string ingest resolves on, and the only join back to upstream
-- rows. Unique because two seeded Drivers sharing one name would make resolution ambiguous in the
-- exact way ADR-0003 exists to prevent.
CREATE TABLE f1.drivers (
    id         uuid PRIMARY KEY,
    full_name  text        NOT NULL UNIQUE,
    short_name text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW()
);

-- Ingested tables key on the upstream's own identifiers and carry no created_by and no deleted_at:
-- this service does not author these rows, and re-ingest updates them rather than deleting
-- (ADR-0002).
CREATE TABLE f1.meetings (
    meeting_key   integer PRIMARY KEY,
    year          integer     NOT NULL,
    name          text        NOT NULL,
    official_name text        NOT NULL,
    circuit_id    uuid        NOT NULL REFERENCES f1.circuits (id),
    country_name  text        NOT NULL,
    location      text        NOT NULL,
    date_start    timestamptz NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT NOW(),
    updated_at    timestamptz NOT NULL DEFAULT NOW()
);

CREATE TABLE f1.sessions (
    session_key  integer PRIMARY KEY,
    meeting_key  integer     NOT NULL REFERENCES f1.meetings (meeting_key),
    circuit_id   uuid        NOT NULL REFERENCES f1.circuits (id),
    session_type text        NOT NULL,
    session_name text        NOT NULL,
    year         integer     NOT NULL,
    date_start   timestamptz NOT NULL,
    date_end     timestamptz NOT NULL,
    is_cancelled boolean     NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT NOW(),
    updated_at   timestamptz NOT NULL DEFAULT NOW()
);

-- Serves the next-Race lookup, which is an ordered scan for the first future Race.
CREATE INDEX sessions_date_start_idx ON f1.sessions (date_start);

-- Serves the corpus scan, which is always "every Race in these seasons".
CREATE INDEX sessions_year_name_idx ON f1.sessions (year, session_name);

-- rainfall is boolean because this source's flag is binary: across all four seasons the only values
-- present are 0 and 1. A numeric column here would imply an intensity the upstream cannot supply.
--
-- The composite primary key IS the index this table needs. Every read is "the samples for one
-- Session", optionally narrowed to a time range within it, and a session-leading btree serves both.
-- A second index would be carried on every ingest write and read by nothing.
CREATE TABLE f1.weather_samples (
    session_key       integer     NOT NULL REFERENCES f1.sessions (session_key),
    observed_at       timestamptz NOT NULL,
    rainfall          boolean     NOT NULL,
    air_temperature   double precision,
    track_temperature double precision,
    humidity          double precision,
    pressure          double precision,
    wind_speed        double precision,
    wind_direction    integer,
    created_at        timestamptz NOT NULL DEFAULT NOW(),
    updated_at        timestamptz NOT NULL DEFAULT NOW(),
    PRIMARY KEY (session_key, observed_at)
);

-- Keyed on the Driver, never on the Racing Number. racing_number is retained because it is what the
-- upstream row carried, but it is not identity: it belongs to a season, and 1 follows the
-- championship, so keying on it merges two people (ADR-0003).
--
-- position is nullable because a Retirement has no position. A sentinel would sort into the
-- classification.
CREATE TABLE f1.session_results (
    session_key    integer     NOT NULL REFERENCES f1.sessions (session_key),
    driver_id      uuid        NOT NULL REFERENCES f1.drivers (id),
    racing_number  integer     NOT NULL,
    position       integer,
    points         double precision NOT NULL DEFAULT 0,
    number_of_laps integer,
    dnf            boolean     NOT NULL DEFAULT false,
    dns            boolean     NOT NULL DEFAULT false,
    dsq            boolean     NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT NOW(),
    updated_at     timestamptz NOT NULL DEFAULT NOW(),
    PRIMARY KEY (session_key, driver_id)
);

-- The primary key already serves "results for one Session". It cannot serve "results for one
-- Driver" because driver_id is its trailing column, and the correlation endpoint attributes wins to
-- Drivers across the whole corpus — so that read gets its own index.
CREATE INDEX session_results_driver_idx ON f1.session_results (driver_id);
