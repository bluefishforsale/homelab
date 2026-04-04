-- Globalview TimescaleDB Schema Initialization
-- Run manually after containers are healthy:
--   docker exec globalview-timescaledb psql -U globalview -d globalview -f /tmp/init-schema.sql
-- Or pipe directly:
--   cat files/globalview-stack/init-schema.sql | docker exec -i globalview-timescaledb psql -U globalview -d globalview

-- Enable extensions
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS postgis;

-- ADS-B positions
CREATE TABLE adsb_positions (
    time        TIMESTAMPTZ NOT NULL,
    icao24      TEXT NOT NULL,
    callsign    TEXT,
    lon         DOUBLE PRECISION,
    lat         DOUBLE PRECISION,
    geo_alt     DOUBLE PRECISION,
    baro_alt    DOUBLE PRECISION,
    velocity    DOUBLE PRECISION,
    heading     DOUBLE PRECISION,
    vert_rate   DOUBLE PRECISION,
    on_ground   BOOLEAN,
    source      TEXT,
    geom        GEOMETRY(Point, 4326)
);
SELECT create_hypertable('adsb_positions', 'time');
CREATE INDEX idx_adsb_geom ON adsb_positions USING GIST (geom);
CREATE INDEX idx_adsb_icao ON adsb_positions (icao24, time DESC);

-- Satellite positions
CREATE TABLE satellite_positions (
    time        TIMESTAMPTZ NOT NULL,
    norad_id    INTEGER NOT NULL,
    name        TEXT,
    lon         DOUBLE PRECISION,
    lat         DOUBLE PRECISION,
    alt_km      DOUBLE PRECISION,
    velocity    DOUBLE PRECISION,
    geom        GEOMETRY(Point, 4326)
);
SELECT create_hypertable('satellite_positions', 'time');
CREATE INDEX idx_sat_geom ON satellite_positions USING GIST (geom);
CREATE INDEX idx_sat_norad ON satellite_positions (norad_id, time DESC);

-- TLE catalog (reference table, not hypertable)
CREATE TABLE tle_catalog (
    norad_id    INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    line1       TEXT NOT NULL,
    line2       TEXT NOT NULL,
    epoch       TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Enable columnstore (required for TimescaleDB 2.x compression)
ALTER TABLE adsb_positions SET (timescaledb.enable_columnstore = true);
ALTER TABLE satellite_positions SET (timescaledb.enable_columnstore = true);

-- Compression policies (enable after 7 days)
SELECT add_compression_policy('adsb_positions', INTERVAL '7 days', if_not_exists => true);
SELECT add_compression_policy('satellite_positions', INTERVAL '7 days', if_not_exists => true);

-- Retention policy (drop raw data after 90 days, keep aggregates)
SELECT add_retention_policy('adsb_positions', INTERVAL '90 days');
SELECT add_retention_policy('satellite_positions', INTERVAL '90 days');

-- Continuous aggregate for timelapse replay (1-minute resolution)
CREATE MATERIALIZED VIEW adsb_positions_1m
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 minute', time) AS bucket,
    icao24,
    last(callsign, time) AS callsign,
    last(lon, time) AS lon,
    last(lat, time) AS lat,
    last(geo_alt, time) AS geo_alt,
    avg(velocity) AS avg_velocity,
    last(heading, time) AS heading
FROM adsb_positions
GROUP BY bucket, icao24;

SELECT add_continuous_aggregate_policy('adsb_positions_1m',
    start_offset => INTERVAL '2 hours',
    end_offset => INTERVAL '1 minute',
    schedule_interval => INTERVAL '1 minute');
