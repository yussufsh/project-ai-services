REGISTRY?=icr.io/ai-services-private
BASE_IMAGE?=$(REGISTRY)/service-base:v0.0.40
CONTAINER_BUILDER?=podman
TEST_PDF?=/path/to/test.pdf

# Build a throwaway image for one docling version and time it.
# Usage:
#   make test DOCLING=2.95.0
#   make test DOCLING=2.100.0
#   ...
#   make test DOCLING=2.127.0
#
# Or run all steps in sequence:
#   make bisect

DOCLING?=2.95.0
REQS=requirements-docling-$(DOCLING).txt
TAG=bisect-docling-$(DOCLING)

.PHONY: build test bisect clean

build:
	$(CONTAINER_BUILDER) build \
		-f Containerfile.bisect \
		--build-arg BASE_IMAGE=$(BASE_IMAGE) \
		--build-arg REQS=$(REQS) \
		-t $(TAG) \
		.

test: build
	@echo "=== Testing docling $(DOCLING) ==="
	$(CONTAINER_BUILDER) run --rm \
		-v $(TEST_PDF):/test.pdf:ro \
		-e DOCLING_MODELS_PATH=/var/docling-models \
		$(TAG) \
		python -c "
import time, sys
sys.path.insert(0, '/services/digitize/parsing')
from converter import get_doc_converter
conv = get_doc_converter()
start = time.time()
result = conv.convert('/test.pdf')
elapsed = time.time() - start
print('docling=$(DOCLING)  pages=%d  time=%.1fs (%.1fmin)' % (len(result.document.pages), elapsed, elapsed/60))
"

bisect:
	@for v in 2.95.0 2.100.0 2.105.0 2.110.0 2.115.0 2.123.0 2.125.0 2.127.0; do \
		$(MAKE) test DOCLING=$$v; \
	done

clean:
	@for v in 2.95.0 2.100.0 2.105.0 2.110.0 2.115.0 2.123.0 2.125.0 2.127.0; do \
		$(CONTAINER_BUILDER) rmi bisect-docling-$$v 2>/dev/null || true; \
	done
