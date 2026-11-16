#!/usr/bin/env bash
set -euo pipefail
image=${1:?Pass the image to verify}
volume="server-domme-smoke-$RANDOM-$RANDOM"
docker volume create "$volume" >/dev/null
cleanup() { docker volume rm "$volume" >/dev/null; }
trap cleanup EXIT

# --help exits before configuration, token use or Discord connection.
docker run --rm --network none "$image" --help
args=(--rm --network none -v "$volume:/usr/project/data")
media="${volume}-media"
docker volume create "$media" >/dev/null
cleanup() { docker volume rm "$volume" "$media" >/dev/null; }
args+=(-v "$media:/usr/project/assets/media")
docker run "${args[@]}" --entrypoint sh "$image" -ec '
    test "$(id -u)" = 10001
    test -r assets/about-banner.webp
    printf persisted > data/container-smoke
    test -s "$TASKS_PATH"
    printf media > assets/media/container-smoke
'
docker run "${args[@]}" --entrypoint sh "$image" -ec '
    test "$(cat data/container-smoke)" = persisted
    test "$(cat assets/media/container-smoke)" = media
'
echo 'Container runtime and persistent storage passed.'
