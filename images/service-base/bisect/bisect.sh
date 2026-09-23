#!/usr/bin/env bash
# Run this INSIDE the service-base container (the known-good v0.0.40 image).
# It upgrades only the docling packages for each step, times ingestion,
# then moves to the next. No image rebuilds needed.
#
# Usage (copy script into a running container):
#   podman cp images/service-base/bisect/bisect.sh <container>:/bisect.sh
#   podman exec -it <container> bash /bisect.sh

set -euo pipefail

DEVPI="https://wheels.developerfirst.ibm.com/ppc64le/linux"
TEST_PDF="${TEST_PDF:-/test.pdf}"
RESULTS="/tmp/bisect-results.txt"

source /var/venv/bin/activate

run_step() {
    local label="$1" docling="$2" core="$3" ibm="$4" parse="$5" rapidocr="$6"

    echo ""
    echo "========================================"
    echo "STEP: $label"
    echo "  docling==$docling  ibm-models==$ibm  parse==$parse  rapidocr==$rapidocr"
    echo "========================================"

    pip install --quiet --no-cache-dir \
        --extra-index-url "$DEVPI" \
        --prefer-binary \
        "docling==$docling" \
        "docling-core==$core" \
        "docling-ibm-models==$ibm" \
        "docling-slim==$docling" \
        "docling-parse==$parse" \
        "rapidocr==$rapidocr"

    echo "Installed:"
    pip show docling docling-core docling-ibm-models docling-parse 2>/dev/null \
        | grep -E "^(Name|Version):" | paste - - | awk '{print "  "$0}'

    elapsed=$(python3 - <<PYEOF
import time, sys, importlib
# Drop cached modules so each step gets a fresh import
for mod in list(sys.modules.keys()):
    if 'docling' in mod or 'converter' in mod:
        del sys.modules[mod]
sys.path.insert(0, '/services/digitize/parsing')
from converter import get_doc_converter
conv = get_doc_converter()
start = time.time()
result = conv.convert('$TEST_PDF')
elapsed = time.time() - start
print("%.1f" % elapsed)
PYEOF
)

    mins=$(echo "$elapsed" | awk '{printf "%.1f", $1/60}')
    echo "RESULT: $label -> ${elapsed}s (${mins} min)"
    echo "$label  docling=$docling  parse=$parse  ibm=$ibm  time=${elapsed}s  (${mins}min)" >> "$RESULTS"
}

# ── Boundary 1: ibm-models 3.13.2 vs 3.15.0 on parse 5.x ───────────────────
# PR #177 (bbox intersection bug fix) landed in ibm-models 3.15.0.
# Test same docling+parse, only ibm-models differs.

# parse 5.x, ibm-models 3.13.2 (known-good v0.0.40 — expected FAST ~5 min)
run_step "A1-docling2.95-ibm3.13.2-parse5"  2.95.0  2.98.0  3.13.2  5.11.0  3.8.1

# parse 5.x, ibm-models 3.15.0 (bbox fix applied — expected SLOW if that's the cause)
run_step "A2-docling2.95-ibm3.15.0-parse5"  2.95.0  2.98.0  3.15.0  5.11.0  3.8.1

# ── Boundary 2: docling-parse 5.x → 6.x (Blend2D renderer) ─────────────────
# Keep ibm-models 3.13.2 to isolate parse change.

run_step "B1-docling2.100-ibm3.13.2-parse6"  2.100.0  2.98.0  3.13.2  6.2.0  3.8.1

# Same parse 6.x but with ibm-models 3.15.0
run_step "B2-docling2.100-ibm3.15.0-parse6"  2.100.0  2.98.0  3.15.0  6.2.0  3.8.1

# ── Boundary 3: docling-parse 6.x → 7.x (full C++ rewrite) ─────────────────
# rapidocr bumps to 3.9.x at docling 2.110.0

run_step "C1-docling2.110-ibm3.13.2-parse7"  2.110.0  2.98.0  3.13.2  7.21.0  3.9.2

# Same parse 7.x but with ibm-models 3.15.0
run_step "C2-docling2.110-ibm3.15.0-parse7"  2.110.0  2.98.0  3.15.0  7.21.0  3.9.2

# ── Boundary 4: ThreadedBackend default (docling 2.123.0) ───────────────────
run_step "D1-docling2.123-ibm3.15.0-parse7"  2.123.0  2.98.0  3.15.0  7.21.0  3.9.2

# ── Boundary 5: ibm-models 3.x → 4.x ───────────────────────────────────────
run_step "E1-docling2.125-ibm4.0.3-parse7"   2.125.0  2.98.0  4.0.3   7.21.0  3.9.2

# ── Final target (PR #1490) ──────────────────────────────────────────────────
run_step "F1-docling2.127-ibm4.0.3-parse7"   2.127.0  2.98.0  4.0.3   7.21.0  3.9.2

echo ""
echo "========================================"
echo "BISECT SUMMARY"
echo "========================================"
cat "$RESULTS"
echo ""
echo "WHAT TO LOOK FOR:"
echo "  A1 fast + A2 slow  -> ibm-models 3.15.0 bbox fix (PR #177) is the cause"
echo "  A1 fast + B1 slow  -> docling-parse 6.x is the cause"
echo "  B1 fast + C1 slow  -> docling-parse 7.x is the cause"
echo "  C1 fast + C2 slow  -> ibm-models 3.15.0 bbox fix is the cause (confirmed on parse 7.x)"
echo "  All Ax/Bx/Cx slow  -> the regression predates parse changes"
