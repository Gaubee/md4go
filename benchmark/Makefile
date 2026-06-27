.PHONY: setup test bench clean

MD4C_REPO := https://github.com/mity/md4c.git
MD4C_DIR  := ../md4c
MD4C_SRC  := $(MD4C_DIR)/src
MD4C_LIB  := $(MD4C_SRC)/libmd4c.a

# ── setup: clone md4c (once) and build static library ─────────────

setup: $(MD4C_LIB)
	@echo "[OK] md4c ready"

$(MD4C_DIR)/.git:
	@echo "[setup] cloning md4c from $(MD4C_REPO)..."
	git clone --depth 1 $(MD4C_REPO) $(MD4C_DIR)

$(MD4C_LIB): $(MD4C_DIR)/.git
	@echo "[setup] building libmd4c.a ..."
	cd $(MD4C_SRC) && cc -c -O2 md4c.c md4c-html.c entity.c && ar rcs libmd4c.a md4c.o md4c-html.o entity.o && rm -f *.o
	@echo "[OK] libmd4c.a built"

# ── test: correctness verification ────────────────────────────────

test: setup
	go test -v -count=1 -timeout=60s ./...

# ── bench: full benchmark suite ───────────────────────────────────

bench: setup
	go test -bench=. -benchtime=3s -benchmem -count=1 ./... | tee result.txt

bench-quick: setup
	go test -bench='Throughput|ParseOnly|GFM|Setup|CGoBridge' -benchtime=1s -benchmem -count=1 ./... | tee result.txt

# ── clean: remove build artifacts ─────────────────────────────────

clean:
	rm -f $(MD4C_LIB)
	@echo "[OK] cleaned"
