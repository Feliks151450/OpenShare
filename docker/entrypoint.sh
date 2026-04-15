#!/bin/sh
set -e
if [ -z "${OPENSHARE_SESSION_SECRET:-}" ] || [ "$OPENSHARE_SESSION_SECRET" = "replace-this-in-local-config" ]; then
  export OPENSHARE_SESSION_SECRET="$(tr -dc 'A-Za-z0-9' </dev/urandom | head -c 48)"
  echo "openshare: OPENSHARE_SESSION_SECRET was unset; generated an ephemeral secret (sessions reset if the container is recreated)." >&2
fi
exec /app/openshare "$@"
