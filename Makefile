# Court build and check tooling (M1.1).
#
#   make tools            install the pinned toolchain into ./.tools/ (no root)
#   make check-<chunk>    run the chunk's checks; one `CHECK <id> <status>` line each
#   make sabotage-<chunk> apply each test/sabotage/<chunk>/*.patch to a scratch
#                         copy and require the check run to go red
#   make regress          frozen-file verification + checks of every DONE chunk
#   make lint             gofmt, go vet, bash -n, spec/progress row linter
#   make clean            remove build artifacts (never removes .tools or evidence)
#   make sysinfo          machine description for measurement reports
#
# Every check/sabotage run is captured under evidence/<chunk>/<UTC timestamp>/
# with the full output and a result.json (command, exit code, duration, commit).

REPO := $(CURDIR)
TOOLS_DIR := $(or $(COURT_TOOLS_DIR),$(REPO)/.tools)
export PATH := $(TOOLS_DIR)/bin:$(PATH)
# Go modules live in the toolchain dir and checks never touch the network.
export GOMODCACHE := $(TOOLS_DIR)/gomod
export GOPROXY := off
export CGO_ENABLED := 0

.PHONY: tools _tools check-% sabotage-% regress lint clean sysinfo

tools:
	@tools/install_tools.sh

# Internal: verify the toolchain is present before anything that needs it.
_tools:
	@tools/install_tools.sh --if-needed

check-%: _tools
	@tools/check.sh $*

sabotage-%: _tools
	@tools/sabotage.sh $*

regress: _tools
	@tools/regress.sh

lint: _tools
	@tools/lint.sh

clean:
	rm -rf bin .work
	git clean -fdq evidence/ 2>/dev/null || true

sysinfo: _tools
	@tools/sysinfo.sh
