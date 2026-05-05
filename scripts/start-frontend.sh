#!/bin/bash
# Auto-restart wrapper for Next.js frontend.
# Logs exit codes and restarts on crash. Clean exit (code 0) stops the loop.
set -u
cd "$(dirname "$0")/../apps/web"
LOG=/tmp/multica-frontend.log

while true; do
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] Starting next start (pid $$)" >> "$LOG"
  REMOTE_API_URL=http://localhost:8080 npx next start --port "${FRONTEND_PORT:-3000}" >> "$LOG" 2>&1
  CODE=$?
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] next exited with code $CODE (signal: $((CODE > 128 ? CODE - 128 : 0)))" >> "$LOG"
  if [ "$CODE" -eq 0 ]; then
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Clean exit, stopping wrapper" >> "$LOG"
    break
  fi
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] Restarting in 3s..." >> "$LOG"
  sleep 3
done
