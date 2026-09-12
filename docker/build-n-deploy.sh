#!/bin/bash

set -euo pipefail

DOCKER_COMPOSE_COMMAND="docker compose -f docker-compose.yml up -d"

# Step 1: Load .env
echo "1. Loading env..."
if [ -f .env ]; then
    source .env
else
    echo "ERROR: .env file not found!"
    exit 1
fi

# Step 2: Pull or verify source code
if [ "${GIT:-}" != "false" ]; then
    echo "2. Cloning repository..."
    rm -rf ./src
    git clone "$GIT_URL" src
else
    if [ ! -d "./src" ]; then
        echo "ERROR: src directory not found!"
        exit 1
    fi
fi

# Step 2b: Seed data files the container reads from the mounted volume.
#
# ./data is mounted over /usr/project/data, so anything baked into the image at
# that path is hidden by the mount — a default has to land on the host instead.
# Only missing files are copied: these are meant to be edited in place, and
# overwriting an operator's character file on every deploy would silently throw
# their work away.
echo "2b. Seeding missing data files..."
mkdir -p ./data
# g4f runs as uid 1000 inside its container and writes cookies and routing
# config here. Created up front because Docker would otherwise make them
# root-owned on first start and the container could not write to them.
mkdir -p ./data/g4f/har_and_cookies ./data/g4f/generated_media
chown -R 1000:1000 ./data/g4f 2>/dev/null ||     echo "   note: could not chown data/g4f — do it by hand if g4f cannot write"
for f in character.md default_task.list.json; do
    if [ -f "./data/$f" ]; then
        echo "   keeping existing data/$f"
    elif [ -f "./src/data/$f" ]; then
        cp "./src/data/$f" "./data/$f"
        echo "   seeded data/$f from source"
    else
        echo "   WARNING: ./src/data/$f not found, nothing to seed"
    fi
done

# Step 3: Bring down running containers
echo "3. Stopping containers..."
docker compose down --remove-orphans

# Step 4: Remove old image(s) related to ALIAS
echo "4. Removing old images..."
OLD_IMAGES=$(docker images --filter=reference="${ALIAS}-image" -q)

if [ -n "$OLD_IMAGES" ]; then
    docker rmi -f $OLD_IMAGES || true
fi

# Step 5: Build image
echo "5. Building new image..."
DOCKER_BUILDKIT=1 docker build -t "${ALIAS}-image" .

# Step 6: Start up containers
echo "6. Starting containers..."
eval "$DOCKER_COMPOSE_COMMAND"

# Step 7: Prune unused Docker junk
echo "7. Cleaning up dangling Docker artifacts..."
docker image prune -f
