#!/bin/sh
set -eu
AUTODIT_APP_PASSWORD="$(cat /run/secrets/app_password)"
export AUTODIT_APP_PASSWORD
psql --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" -v ON_ERROR_STOP=1 <<'SQL'
\getenv app_password AUTODIT_APP_PASSWORD
create role autodit_app login password :'app_password';
SQL
unset AUTODIT_APP_PASSWORD
