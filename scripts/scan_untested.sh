#!/bin/bash
# Scan for Go packages with source files but no test files

BASE="/mnt/e/test/rewrite-agently/inferglow-github"

for d in action approval audit builtins cli components context desktop eval examples flow imbridge mcpserver memory messagebus model observability orchestrator rag rerank resource sandbox schema security server session skill storage workspace; do
    dir="$BASE/$d"
    if [ -d "$dir" ]; then
        find "$dir" -type d -not -path "*/vendor/*" 2>/dev/null | while IFS= read -r subdir; do
            rel="${subdir#$BASE/}"
            src_count=$(find "$subdir" -maxdepth 1 -name "*.go" -not -name "*_test.go" 2>/dev/null | wc -l)
            tst_count=$(find "$subdir" -maxdepth 1 -name "*_test.go" 2>/dev/null | wc -l)
            if [ "$src_count" -gt 0 ] && [ "$tst_count" -eq 0 ]; then
                echo "UNTESTED: $rel"
            fi
        done
    fi
done

echo ""
echo "--- Source file counts for untested packages ---"
for p in cli/cmd/inferglow-cli context/compress context/store context/store/postgres context/store/redis context/tools flow/stage/builtin model/internal/ssestream observability orchestrator/agent/internal/turnloop sandbox/cmd/sandbox server/cmd/inferglow-server server/config server/handler server/middleware; do
    count=$(find "$BASE/$p" -maxdepth 1 -name "*.go" -not -name "*_test.go" 2>/dev/null | wc -l)
    if [ "$count" -gt 0 ]; then
        echo "  $p: $count source files"
    fi
done