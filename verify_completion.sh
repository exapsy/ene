#!/bin/bash

# ENE Completion Verification Script
# Tests that all completion features are working correctly

set -e

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "🧪 ENE Completion Verification Script"
echo "======================================"
echo

# Test 1: Verify ENE binary is installed
echo -n "1. Checking ENE binary... "
if command -v ene >/dev/null 2>&1; then
    echo -e "${GREEN}✓${NC}"
else
    echo -e "${RED}✗ ENE binary not found in PATH${NC}"
    echo "Please run: make install"
    exit 1
fi

# Test 2: Verify completion file exists
echo -n "2. Checking completion file... "
if [ -f ~/.local/share/zsh/completions/_ene ]; then
    echo -e "${GREEN}✓${NC}"
else
    echo -e "${RED}✗ Completion file not found${NC}"
    echo "Please run: make completion-zsh"
    exit 1
fi

# Test 3: Verify shell configuration
echo -n "3. Checking shell configuration... "
if grep -q "fpath=.*\.local/share/zsh/completions" ~/.zshrc 2>/dev/null; then
    echo -e "${GREEN}✓${NC}"
else
    echo -e "${YELLOW}⚠ fpath not configured in ~/.zshrc${NC}"
    echo "Please run: make completion-zsh"
fi

# Test 4: Check list-suites command
echo -n "4. Testing list-suites command... "
if ene list-suites >/dev/null 2>&1; then
    suite_count=$(ene list-suites 2>/dev/null | grep -c "^  " || echo "0")
    echo -e "${GREEN}✓ (found $suite_count suites)${NC}"
else
    echo -e "${YELLOW}⚠ No test suites found${NC}"
    echo "   Create some test suites with: ene scaffold-test <name>"
fi

# Test 5: Display available suites
echo
echo "📋 Available Test Suites:"
if ene list-suites 2>/dev/null | grep -q "^  "; then
    ene list-suites | grep "^  " | while read -r line; do
        echo "   • $line"
    done
else
    echo "   (none found - create some with 'ene scaffold-test <name>')"
fi

# Test 6: Show completion capabilities
echo
echo "🎯 Completion Capabilities:"
echo "   • Command completion: ene <TAB>"
echo "   • Flag completion: ene --<TAB>"
echo "   • Single suite: ene --suite <TAB>"
echo "   • Multiple suites: ene --suite suite1,<TAB>"
echo "   • Partial matching: ene --suite api<TAB>"

# Test 7: Demonstrate multi-suite syntax
echo
echo "💡 Multi-Suite Examples:"
if ene list-suites 2>/dev/null | grep -q "^  "; then
    suites=($(ene list-suites 2>/dev/null | grep "^  " | head -3 | tr -d ' '))
    if [ ${#suites[@]} -ge 2 ]; then
        echo "   ene --suite=${suites[0]},${suites[1]}"
        echo "   ene --suite=integration,api"
        echo "   ene --suite=mock,unit,test"
    fi
else
    echo "   ene --suite=suite1,suite2,suite3"
    echo "   ene --suite=integration,api"
fi

# Test 8: Verify command list
echo
echo "📚 Available Commands:"
ene --help | grep -E '^  [a-z]' | grep -v '^  ene' | while read -r line; do
    cmd=$(echo "$line" | awk '{print $1}')
    desc=$(echo "$line" | cut -d' ' -f2-)
    echo "   • $cmd - $desc"
done

# Test 9: Verify --suite flag completion
echo
echo "🎯 Testing --suite Flag Completion:"
if ene list-suites 2>/dev/null | grep -q "^  "; then
    echo -n "   • Empty completion: "
    suite_completion=$(ene __complete --suite "" 2>&1 | grep -v "Completion ended" | grep -v "^:" | grep "." | wc -l)
    available_suites=$(ene list-suites | grep -c "^  ")
    if [ "$suite_completion" -eq "$available_suites" ]; then
        echo -e "${GREEN}✓${NC} (returns all $available_suites suites)"
    else
        echo -e "${RED}✗${NC} (expected $available_suites, got $suite_completion)"
    fi
    
    echo -n "   • Partial completion (api): "
    api_completion=$(ene __complete --suite "api" 2>&1 | grep -v "Completion ended" | grep -v "^:" | grep "api")
    if [ -n "$api_completion" ]; then
        echo -e "${GREEN}✓${NC} (found: $(echo "$api_completion" | tr '\n' ' '))"
    else
        echo -e "${YELLOW}⚠${NC} (no api-prefixed suites found)"
    fi
    
    echo -n "   • Multi-suite completion: "
    multi_completion=$(ene __complete --suite "mock-tests," 2>&1 | grep -v "Completion ended" | grep -v "^:" | head -1)
    if [[ "$multi_completion" == mock-tests,* ]]; then
        echo -e "${GREEN}✓${NC} (prefix preserved)"
    else
        echo -e "${RED}✗${NC} (prefix not preserved correctly)"
    fi
else
    echo "   • No test suites available for completion testing"
fi

echo
echo "✅ Completion verification complete!"
echo
echo "🚀 To test interactive completion:"
echo "   1. Restart your shell: exec zsh"
echo "   2. Try: ene <TAB>"
echo "   3. Try: ene --suite <TAB>"
echo "   4. Try: ene --suite api<TAB>"
echo "   5. Try: ene --suite mock-tests,<TAB>"
echo "   6. Try: ene list-<TAB>"
echo
echo "📖 For more examples, see: completion_demo.md"