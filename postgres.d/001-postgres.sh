#!/bin/bash

set -euo pipefail

cat >> /var/lib/postgresql/data/postgresql.conf <<EOF
log_destination = 'stderr'
logging_collector = on
log_directory = 'pg_log'
log_statement = 'all'
EOF

mkdir -p /var/lib/postgresql/data/pg_log
chown -R postgres:postgres /var/lib/postgresql/data/pg_log
