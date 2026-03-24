#!/usr/bin/env bash
# pii-guard.sh — Scan staged git diffs for PII before commit
# Usage: scripts/pii-guard.sh          (as pre-commit hook)
#        scripts/pii-guard.sh <file>    (scan a specific file)
#
# Exit 0 = clean, Exit 1 = PII found
# Respects .pii-allowlist (one pattern per line, anchored grep -F)

set -euo pipefail

RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m'

# --- Allowlist ---
ALLOWLIST_FILE=".pii-allowlist"
load_allowlist() {
  if [[ -f "$ALLOWLIST_FILE" ]]; then
    grep -v '^#' "$ALLOWLIST_FILE" | grep -v '^\s*$' || true
  fi
}

is_allowed() {
  local line="$1"
  while IFS= read -r pattern; do
    if [[ -n "$pattern" ]] && echo "$line" | grep -qF "$pattern"; then
      return 0
    fi
  done <<< "$(load_allowlist)"
  return 1
}

# --- PII patterns ---
# Each entry: "label|regex"
PII_PATTERNS=(
  # US phone numbers (10+ digits with optional country code, parens, dashes, dots, spaces)
  "US Phone Number|\+?1?[-. (]*[2-9][0-9]{2}[-. )]*[2-9][0-9]{2}[-. ]*[0-9]{4}"
  # E.164 international phone (11-15 digits)
  "E.164 Phone|\+[1-9][0-9]{10,14}"
  # Email addresses
  "Email Address|[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}"
  # SSN (xxx-xx-xxxx)
  "SSN|[0-9]{3}-[0-9]{2}-[0-9]{4}"
  # Credit card (13-19 digits, optionally grouped)
  "Credit Card|[0-9]{4}[-. ]?[0-9]{4}[-. ]?[0-9]{4}[-. ]?[0-9]{1,7}"
  # AWS keys
  "AWS Access Key|AKIA[0-9A-Z]{16}"
  # Generic API key patterns (long hex/base64 after common prefixes)
  "API Key|(api[_-]?key|apikey|secret|token|password)[[:space:]]*[:=][[:space:]]*['\"]?[A-Za-z0-9/+=_-]{20,}"
  # Private keys
  "Private Key|-----BEGIN (RSA |EC |DSA )?PRIVATE KEY-----"
)

# Patterns to SKIP (test fixtures, examples, docs with dummy data)
SKIP_FILE_PATTERNS=(
  "*.test.*"
  "*.spec.*"
  "__tests__"
  "fixtures"
)

should_skip_file() {
  local file="$1"
  for pattern in "${SKIP_FILE_PATTERNS[@]}"; do
    if [[ "$file" == *"$pattern"* ]]; then
      return 0
    fi
  done
  return 1
}

# --- Main scan ---
found=0
scan_line() {
  local file="$1"
  local lineno="$2"
  local line="$3"

  for entry in "${PII_PATTERNS[@]}"; do
    local label="${entry%%|*}"
    local regex="${entry#*|}"

    if echo "$line" | grep -qEi -- "$regex"; then
      # Check allowlist
      if is_allowed "$line"; then
        continue
      fi
      # Skip dummy/example data
      if echo "$line" | grep -qiE '555[-. ]?[0-9]{4}|example\.com|test@|dummy|placeholder|xxx|000-00-0000'; then
        continue
      fi
      echo -e "${RED}PII DETECTED${NC} [${YELLOW}${label}${NC}] in ${file}:${lineno}"
      echo "  $line"
      echo ""
      found=1
    fi
  done
}

if [[ $# -gt 0 ]]; then
  # Scan a specific file
  file="$1"
  lineno=0
  while IFS= read -r line; do
    lineno=$((lineno + 1))
    scan_line "$file" "$lineno" "$line"
  done < "$file"
else
  # Scan staged git diff (added lines only)
  git diff --cached --diff-filter=ACMR --name-only 2>/dev/null | while IFS= read -r file; do
    # Skip binary files
    if file "$file" | grep -q "binary"; then
      continue
    fi
    # Skip test files
    if should_skip_file "$file"; then
      continue
    fi
    # Only scan added/modified lines (+ lines in diff)
    git diff --cached -U0 -- "$file" 2>/dev/null | grep '^+' | grep -v '^+++' | while IFS= read -r diffline; do
      line="${diffline:1}" # strip leading +
      scan_line "$file" "staged" "$line"
    done
  done
fi

if [[ $found -gt 0 ]]; then
  echo -e "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo -e "${RED}COMMIT BLOCKED: PII detected in staged changes.${NC}"
  echo -e "  • Add false positives to ${YELLOW}.pii-allowlist${NC}"
  echo -e "  • Or use ${YELLOW}git commit --no-verify${NC} to bypass"
  echo -e "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  exit 1
fi

exit 0
