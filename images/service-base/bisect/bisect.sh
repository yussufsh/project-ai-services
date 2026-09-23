#!/usr/bin/env bash
# Run this INSIDE the service-base container (the known-good v0.0.40 image).
# It upgrades only the 4 docling packages for each step, times ingestion,
# then moves to the next. No image rebuilds needed.
#
# Usage:
#   podman run --rm -it \
#     -v /path/to/test.pdf:/test.pdf:ro \
#     icr.io/ai-services-private/service-base:v0.0.40 \
#     bash /bisect/bisect.sh
#
# Or copy into a running container:
#   podman cp bisect.sh <container>:/bisect.sh && podman exec -it <container> bash /bisect.sh

set -euo pipefail

DEVPI="https://wheels.developerfirst.ibm.com/ppc64le/linux"
TEST_PDF="${TEST_PDF:-/test.pdf}"
RESULTS="/tmp/bisect-results.txt"

source /var/venv/bin/activate

run_step() {
    local label="$1" docling="$2" core="$3" ibm="$4" slim="$5" parse="$6"

    echo ""
    echo "========================================"
    echo "STEP: $label"
    echo "  docling==$docling  core==$core  ibm-models==$ibm  parse==$parse"
    echo "========================================"

    pip install --quiet --no-cache-dir \
        --extra-index-url "$DEVPI" \
        --prefer-binary \
        "docling==$docling" \
        "docling-core==$core" \
        "docling-ibm-models==$ibm" \
        "docling-slim==$slim" \
        "docling-parse==$parse"

    echo "Installed:"
    pip show docling docling-core docling-ibm-models docling-parse 2>/dev/null \
        | grep -E "^(Name|Version):" | paste - - | awk '{print "  "$0}'

    elapsed=$(python3 - <<PYEOF
import time, sys
sys.path.insert(0, '/services/digitize/parsing')
# Fresh import each time
import importlib
if 'converter' in sys.modules:
    del sys.modules['converter']

from converter import get_doc_converter
conv = get_doc_converter()
start = time.time()
result = conv.convert('$TEST_PDF')
elapsed = time.time() - start
pages = len(list(result.document.pages))
print("%.1f" % elapsed)
PYEOF
)

    mins=$(echo "$elapsed" | awk '{printf "%.1f", $1/60}')
    echo "RESULT: $label -> ${elapsed}s (${mins} min)"
    echo "$label  docling=$docling  parse=$parse  ibm=$ibm  time=${elapsed}s  (${mins}min)" >> "$RESULTS"
}

# Step 1: baseline — matches known-good v0.0.40 container exactly
run_step "step1-baseline"  2.95.0  2.98.0 3.15.0 2.95.0  5.11.0

# Step 2: docling-parse jumps from 5.x -> 6.x (first version requiring 6.x)
run_step "step2-parse6x"   2.100.0 2.98.0 3.15.0 2.100.0 6.2.0

# Step 3: still parse 6.x, slightly newer docling
run_step "step3-parse6x-2" 2.105.0 2.98.0 3.15.0 2.105.0 6.2.0

# Step 4: docling-parse jumps from 6.x -> 7.x (full C++ rewrite)
run_step "step4-parse7x"   2.110.0 2.98.0 3.15.0 2.110.0 7.21.0

# Step 5: still parse 7.x, same ibm-models
run_step "step5-parse7x-2" 2.115.0 2.98.0 3.15.0 2.115.0 7.21.0

# Step 6: ThreadedDoclingParseDocumentBackend becomes default (docling 2.123.0)
run_step "step6-threaded"  2.123.0 2.98.0 3.15.0 2.123.0 7.21.0

# Step 7: ibm-models jumps from 3.x -> 4.x (TableFormer v2 post-proc fixes)
run_step "step7-ibm4x"     2.125.0 2.98.0 4.0.3  2.125.0 7.21.0

# Step 8: final target (PR #1490)
run_step "step8-final"     2.127.0 2.98.0 4.0.3  2.127.0 7.21.0

echo ""
echo "========================================"
echo "BISECT SUMMARY"
echo "========================================"
cat "$RESULTS"
