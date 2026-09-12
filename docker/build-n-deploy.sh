#!/bin/bash

set -euo pipefail

DOCKER_COMPOSE_COMMAND="docker compose -f docker-compose.yml up -d"

# Step 1: Load the few settings this script needs.
#
# Read, not sourced. `source .env` executes every line as shell, so a value
# containing a pipe is parsed as a pipeline: CHAT_BACKENDS entries are
# name|url|model by design, and sourcing one produced "http://g4f:8080/v1: No
# such file or directory" and killed the deploy. Docker Compose reads this same
# file literally, and so does this.
#
# Only ALIAS, GIT and GIT_URL are needed here. Everything else in .env is for
# the container, and compose passes it through itself.
echo "1. Loading env..."
if [ ! -f .env ]; then
    echo "ERROR: .env file not found!"
    exit 1
fi

read_env() {
    sed -n "s/^[[:space:]]*$1=//p" .env | tail -n 1 | sed -e 's/^"//' -e 's/"$//' -e "s/^'//" -e "s/'\$//"
}

ALIAS=$(read_env ALIAS)
GIT=$(read_env GIT)
GIT_URL=$(read_env GIT_URL)

# Exported so every `docker compose` call below sees the same profiles.
#
# It has to be set before `compose down` as well as before `up`, not only for
# symmetry: a service whose profile is inactive is not "stopped" by down, it is
# an orphan, and --remove-orphans deletes it. Starting the g4f profile and then
# redeploying without this would take the container away again.
COMPOSE_PROFILES=$(read_env COMPOSE_PROFILES)
export COMPOSE_PROFILES

if [ -n "$COMPOSE_PROFILES" ]; then
    echo "   profiles: $COMPOSE_PROFILES"
fi

if [ -z "$ALIAS" ]; then
    echo "ERROR: ALIAS is not set in .env — it names the image and the container."
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
if ! chown -R 1000:1000 ./data/g4f 2>/dev/null; then
    echo "   note: could not chown data/g4f — do it by hand if g4f cannot write"
fi
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
