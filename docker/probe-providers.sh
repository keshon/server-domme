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
# It asks twice. A backend that answers "say OK" has proved almost nothing —
# pollinations served a two-word prompt and refused a real one with 402
# KEY_BUDGET_EXHAUSTED — so whatever survives the first pass is asked again
# with a prompt the size of a real character, and only those go in the output.
#
# Whatever it prints goes in CHAT_BACKENDS. More than one entry is the point:
# a provider that works today is the one answering 403 next week, and the pool
# fails over between them.

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

# A 200 is not an answer. Several of these providers are image or audio models
# that cheerfully return a markdown image or an <audio> tag for any prompt, and
# one returns "Sign up and repeat your request." with a perfectly good status
# code. Either in the pool means the bot posts a picture instead of speaking.
MEDIA_MARKERS = ("![", "<audio", "<video", "<img", "](https://image.")
BRUSH_OFF = ("sign up", "log in", "login required", "create an account",
             "subscribe", "verify you", "authentication")


def classify(reply):
    text = reply.strip()
    if not text:
        return "EMPTY"
    if any(marker in text[:120] for marker in MEDIA_MARKERS):
        return "NOT TEXT"
    lowered = text.lower()
    if len(text) < 200 and any(phrase in lowered for phrase in BRUSH_OFF):
        return "SUSPECT"
    return "WORKS"


def ask(model_id, messages, timeout):
    body = json.dumps({
        "model": model_id,
        "messages": messages,
        "stream": False,
    }).encode()
    req = urllib.request.Request(
        BASE + "/chat/completions", body, {"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            reply = json.load(resp)["choices"][0]["message"]["content"]
            return classify(reply), reply[:55].replace("\n", " ")
    except urllib.error.HTTPError as err:
        detail = err.read()[:70].decode("utf-8", "replace").replace("\n", " ")
        return "HTTP %d" % err.code, detail
    except Exception as err:  # noqa: BLE001 - any failure is a failure to report
        return "FAIL", str(err)[:70].replace("\n", " ")


HELLO = [{"role": "user", "content": "say OK"}]

# Roughly the size of a real assembled character prompt, which is about 2700
# characters of persona, limits and examples before anyone has said anything.
FILLER = ("You are a long-standing member of this server, not an assistant. "
          "You speak briefly, you decline freely, and you never offer help "
          "nobody asked for. ") * 12
REAL = [
    {"role": "system", "content": FILLER},
    {"role": "user", "content": "cass: @domme you awake"},
    {"role": "assistant", "content": "unfortunately"},
    {"role": "user", "content": "cass: what do you make of the new rules"},
]

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

print("%d providers, %d models - trying %d\n" % (len(providers), len(models), len(targets)),
      flush=True)

candidates = []
for name in targets:
    status, detail = ask(name, HELLO, TIMEOUT)
    print("  %-9s %-32s %s" % (status, name, detail), flush=True)
    if status == "WORKS":
        candidates.append(name)

if not candidates:
    print("\nNothing answered with text. The providers are reachable from a"
          " browser and not from here, which is the same wall the hosted relay"
          " hit.", flush=True)
    raise SystemExit(0)

print("\n%d answered with text. Re-testing at the size of a real character"
      " prompt:\n" % len(candidates), flush=True)

survivors = []
for name in candidates:
    status, detail = ask(name, REAL, TIMEOUT * 2)
    print("  %-9s %-32s %s" % (status, name, detail), flush=True)
    if status == "WORKS":
        survivors.append(name)

print(flush=True)
if not survivors:
    print("All of them answered a two-word prompt and none carried a real one."
          " That is a size limit, not an outage.", flush=True)
    raise SystemExit(0)

entries = ",".join(
    "g4f-%d|http://g4f:8080/v1|%s" % (i, name)
    for i, name in enumerate(survivors, 1))
print("%d carried a full prompt, all of them:\n" % len(survivors), flush=True)
print('  CHAT_BACKENDS="%s"\n' % entries, flush=True)
PY
