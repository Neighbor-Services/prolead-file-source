#!/usr/bin/env bash
set -e

echo "=================================================="
echo "🚀 Running GoStore / Prolead File Complete Test Suite"
echo "=================================================="

# 1. Backend Core & Service Tests
echo ""
echo "🔹 [1/4] Running Go Backend Unit & Integration Tests..."
go test -v ./internal/...

# 2. Go Client SDK Tests
echo ""
echo "🔹 [2/4] Running Go Client SDK Tests..."
cd packages/go_prolead_file
go test -v ./...
cd ../..

# 3. TypeScript Client SDK
echo ""
echo "🔹 [3/5] Building & Testing TypeScript Client SDK..."
cd packages/ts_prolead_file
if [ -f "package.json" ]; then
  npm run build
  npm test
fi
cd ../..

# 4. Flutter Client SDK Tests
echo ""
echo "🔹 [4/5] Running Flutter Client SDK Tests..."
cd packages/flutter_prolead_file
if command -v flutter >/dev/null 2>&1; then
  flutter test
else
  echo "Flutter not installed in PATH, skipping Flutter SDK tests."
fi
cd ../..

# 5. Angular Frontend Build & Static Sync
echo ""
echo "🔹 [5/5] Validating Angular 22 Studio Build..."
cd frontend
npm run build
cd ..

echo ""
echo "=================================================="
echo "✅ All GoStore Tests & Builds Succeeded (100% Pass)!"
echo "=================================================="

