FROM postgres:16-alpine

COPY internal/database/migrations/*.sql /migrations/
COPY migrations-entrypoint.sh /migrations/migrate.sh
RUN chmod +x /migrations/migrate.sh

ENTRYPOINT ["/bin/sh", "/migrations/migrate.sh"]
