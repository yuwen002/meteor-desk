#!/usr/bin/env bash
# 汇总 Go 测试覆盖率：打印总覆盖率、分包明细以及相对基线的变化，
# 并在总覆盖率低于下限时阻断合并。
#
# 用法：
#   go test ./... -covermode=atomic -coverprofile=coverage.out
#   bash scripts/coverage-check.sh coverage.out                     # CI / 本地校验
#   bash scripts/coverage-check.sh coverage.out --update-baseline   # 刷新基线文件
#
# 可通过环境变量调整：
#   COVERAGE_FLOOR            总覆盖率下限，默认 70（百分点）
#   COVERAGE_DROP_TOLERANCE   单包允许下降的幅度，默认 2（超出只告警，不阻断）
#   COVERAGE_BASELINE         基线文件路径，默认 .github/coverage-baseline.txt
set -euo pipefail

profile="${1:-coverage.out}"
mode="${2:-check}"
baseline="${COVERAGE_BASELINE:-.github/coverage-baseline.txt}"
floor="${COVERAGE_FLOOR:-70}"
tolerance="${COVERAGE_DROP_TOLERANCE:-2}"

if [[ ! -f "$profile" ]]; then
  echo "::error::未找到覆盖率文件 $profile，请先执行 go test ./... -coverprofile=$profile"
  exit 1
fi

# profile 每行形如「模块路径/包名/文件.go:起行.起列,止行.止列 语句数 命中次数」。
# 先剔除 frontend 与 node_modules 下的条目：`go test ./...` 会把依赖目录里带 go 源码的包（例如
# flatted 的 golang 包）一并收进 profile，它们没有测试、会拉低总覆盖率，且本地是否命中取决于
# 开发者有没有先跑 npm ci，不能当成门禁依据。覆盖率只统计仓库自身的 Go 代码。
filtered="$(
  awk 'NR == 1 { next } NF >= 3 && $1 !~ /\/(frontend|node_modules)\// { print }' "$profile"
)"

# 去掉文件名即可按包聚合；命中次数大于 0 的语句计入已覆盖。
current="$(
  printf '%s\n' "$filtered" | awk '
    {
      split($1, loc, ":")
      pkg = loc[1]
      sub(/\/[^/]+\.go$/, "", pkg)
      stmts[pkg] += $2
      if ($3 + 0 > 0) covered[pkg] += $2
    }
    END {
      for (pkg in stmts) {
        pct = stmts[pkg] > 0 ? covered[pkg] * 100 / stmts[pkg] : 0
        printf "%s\t%.1f\n", pkg, pct
      }
    }
  ' | sort
)"

if [[ "$mode" == "--update-baseline" ]]; then
  mkdir -p "$(dirname "$baseline")"
  {
    echo "# Go 测试覆盖率基线：由 bash scripts/coverage-check.sh coverage.out --update-baseline 生成。"
    echo "# 每行「包路径<TAB>覆盖率」，CI 用它打印分包变化明细；总覆盖率下限见 COVERAGE_FLOOR。"
    echo "$current"
  } > "$baseline"
  echo "已刷新覆盖率基线：$baseline"
  exit 0
fi

# 总覆盖率同样基于剔除后的条目累加，与分包明细保持同一口径（go tool cover 无法排除路径）。
total="$(
  printf '%s\n' "$filtered" | awk '
    { s += $2; if ($3 + 0 > 0) c += $2 }
    END { if (s > 0) printf "%.1f", c * 100 / s; else printf "0.0" }
  '
)"

echo "总覆盖率：${total}%（下限 ${floor}%）"
echo "分包明细（当前 / 基线 / 相对基线变化）："
awk -F'\t' -v tolerance="$tolerance" '
  NR == FNR {
    if ($0 !~ /^#/ && NF >= 2) { base[$1] = $2; known[$1] = 1 }
    next
  }
  NF >= 2 { cur[$1] = $2; order[++n] = $1; seen[$1] = 1 }
  END {
    for (i = 1; i <= n; i++) {
      pkg = order[i]
      if (pkg in base) {
        delta = cur[pkg] - base[pkg]
        if (delta > 0.05) mark = sprintf("上升 %.1f", delta)
        else if (delta < -0.05) mark = sprintf("下降 %.1f", -delta)
        else mark = "持平"
        printf "  %-46s %6.1f%% %6.1f%%  %s\n", pkg, cur[pkg], base[pkg], mark
        if (delta < -tolerance)
          printf "::warning::%s 覆盖率明显下降：基线 %.1f%% → 当前 %.1f%%，请确认是否漏测\n", pkg, base[pkg], cur[pkg]
      } else {
        printf "  %-46s %6.1f%% %6s  新增包（已写入基线建议）\n", pkg, cur[pkg], "-"
      }
    }
    for (pkg in known) {
      if (!(pkg in seen)) printf "  %-46s %6s %6.1f%%  包已不存在，请同步基线\n", pkg, "-", base[pkg]
    }
  }
' "$baseline" <(echo "$current")

below_floor="$(awk -v total="$total" -v floor="$floor" 'BEGIN { print (total + 0 < floor + 0) ? 1 : 0 }')"
if [[ "$below_floor" == "1" ]]; then
  echo "::error::总覆盖率 ${total}% 低于下限 ${floor}%，请补齐测试后再提交。"
  exit 1
fi
echo "覆盖率门禁通过（${total}% ≥ ${floor}%）。"
