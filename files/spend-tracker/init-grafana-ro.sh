#!/bin/sh
# Read-only role for the Grafana datasource. Grafana lets any editor run raw
# SQL through a datasource, and POSTGRES_USER is a superuser, so handing Grafana
# that login would let a dashboard query run COPY ... PROGRAM in this container.
#
# NOTE: the entrypoint runs this once, when the data directory is first
# created. Rotating spend_tracker.grafana_ro_password in the vault does not
# change the role; run ALTER ROLE grafana_ro PASSWORD by hand as well, or
# Grafana is locked out on its next restart. For the same reason a failure here
# is not retried: the data directory exists by then, so the next start skips
# this script and Postgres comes up healthy without the role. Create it by hand
# or remove pgdata and let the collector backfill again.
set -eu

psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v pw="$GRAFANA_RO_PASSWORD" -v db="$POSTGRES_DB" <<'SQL'
CREATE ROLE grafana_ro LOGIN PASSWORD :'pw';
GRANT CONNECT ON DATABASE :"db" TO grafana_ro;
GRANT USAGE ON SCHEMA public TO grafana_ro;
-- The collector creates the table later, as this same user, so the grant has
-- to be a default privilege rather than a GRANT on a table that isn't here yet.
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO grafana_ro;
SQL
