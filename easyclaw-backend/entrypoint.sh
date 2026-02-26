#!/bin/sh
CMD=$1
shift

exec /app/"$CMD" "$@"
