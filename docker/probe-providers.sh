#!/bin/bash
#
# Ask the self-hosted g4f container which of its providers actually answer.
#
#   ./probe-providers.sh                 # every provider, one request each
#   ./probe-providers.sh models          # named models instead of providers
#   ./probe-providers.sh all             # both
#   TIMEOUT=40 ./probe-providers.sh      # slower providers get longer
#
# g4f lists what it believes works, which is not the same as what answers from
# this host today: a provider can be listed and still return 402 because the
# model behind it is paywalled, or 403 because the request came from a
# datacentre address. The only way to know is to ask, which is what this does.
#
# Whatever comes back WORKS goes in CHAT_BACKENDS, most preferred first:
#
#   CHAT_BACKENDS="g4f-a|http://g4f:8080/v1|<first>,g4f-b|http://g4f:8080/v1|<second>"
#
# More than one entry is the point. A provider that works today is the one
# answering 403 next week, and the pool fails over between them.

set -euo pipefail

# Run from this script's directory so docker-compose.yml is found wherever it
# was invoked from.
cd "$(dirname "$0")"

WHAT="${1:-providers}"
TIMEOUT="${TIMEOUT:-20}"

case "$WHAT" in
    providers | models | all) ;;
    *)
        echo "usage: $0 [providers|models|all]" >&2
        exit 2
        ;;
esac

if ! docker compose ps --status running --services 2>/dev/null | grep -qx g4f; then
    echo "ERROR: the g4f service is not running." >&2
    echo "       It sits behind a compose profile — set COMPOSE_PROFILES=g4f in .env," >&2
    echo "       then: docker compose up -d g4f" >&2
    exit 1
fi

echo "Probing $WHAT with a ${TIMEOUT}s timeout each. Ctrl-C once you have enough."
echo

# -T because exec allocates a TTY by default, and a TTY cannot also be fed a
# heredoc: "the input device is not a TTY".
docker compose exec -T g4f python - "$WHAT" "$TIMEOUT" <<'PY'
import json
import sys
import urllib.error
import urllib.request

BASE = "http://127.0.0.1:8080/v1"
WHAT = sys.argv[1]
TIMEOUT = int(sys.argv[2])


def ask(model_id):
    body = json.dumps({
        "model": model_id,
        "messages": [{"role": "user", "content": "say OK"}],
        "stream": False,
    }).encode()
    req = urllib.request.Request(
        BASE + "/chat/completions", body, {"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
            reply = json.load(resp)["choices"][0]["message"]["content"]
            return "WORKS", reply[:55].replace("\n", " ")
    except urllib.error.HTTPError as err:
        detail = err.read()[:70].decode("utf-8", "replace").replace("\n", " ")
        return f"HTTP {err.code}", detail
    except Exception as err:  # noqa: BLE001 - any failure is a failure to report
        return "FAIL", str(err)[:70].replace("\n", " ")


with urllib.request.urlopen(BASE + "/models", timeout=30) as resp:
    entries = json.load(resp)["data"]

# /v1/models returns both: entries flagged provider=true are provider names,
# which are themselves valid as a "model" and resolve to that provider's own
# default. That is the way past a paywalled model name like gpt-4o-mini.
providers = [e["id"] for e in entries if e.get("provider")]
models = [e["id"] for e in entries if not e.get("provider")]

targets = []
if WHAT in ("providers", "all"):
    targets += providers
if WHAT in ("models", "all"):
    targets += models

print(f"{len(providers)} providers, {len(models)} models — trying {len(targets)}\n", flush=True)

working = []
for name in targets:
    status, detail = ask(name)
    print(f"  {status:9s} {name:32s} {detail}", flush=True)
    if status == "WORKS":
        working.append(name)

print(flush=True)
if working:
    print(f"{len(working)} answered:", flush=True)
    entries = ",".join(
        f"g4f-{i}|http://g4f:8080/v1|{name}" for i, name in enumerate(working[:3], 1))
    print(f'\n  CHAT_BACKENDS="{entries}"\n', flush=True)
else:
    print("Nothing answered. The providers are reachable from a browser and not"
          " from here, which is the same wall the hosted relay hit.", flush=True)
PY
