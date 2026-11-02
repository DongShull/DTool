#!/bin/sh
cd "${WORKDIR:-/app}" || exit

# 新发布包把二进制放在归档根目录；旧发布包在 dist/linux/<arch>/ 下。
new_bin=""
if [ -f "./DTool-new" ]; then
    new_bin="./DTool-new"
else
    for candidate in ./dist/linux/*/DTool-new; do
        if [ -f "$candidate" ]; then
            new_bin="$candidate"
            break
        fi
    done
fi

if [ -n "$new_bin" ]; then
    # 换入新版本前保留上一版（DTool.bak），供启动失败时自动回退。
    # 必须"写临时文件 → 校验大小一致 → 原子 mv"：直接 cp 到 DTool.bak 时，
    # 若复制过程中被 docker stop / OOM / 断电打断，会留下截断的备份，
    # 回退时把坏副本换上去就再也起不来了。
    if [ -f ./DTool ]; then
        if cp ./DTool ./DTool.bak.tmp 2>/dev/null; then
            size_src=$(wc -c < ./DTool 2>/dev/null || echo 0)
            size_dst=$(wc -c < ./DTool.bak.tmp 2>/dev/null || echo 0)
            if [ "$size_src" = "$size_dst" ] && [ "$size_dst" != "0" ]; then
                mv ./DTool.bak.tmp ./DTool.bak
            else
                echo "warning: DTool.bak 备份不完整，已丢弃（保留原 DTool.bak）" >&2
                rm -f ./DTool.bak.tmp
            fi
        else
            echo "warning: 无法备份当前二进制，回退保护降级" >&2
        fi
    fi
    mv "$new_bin" ./DTool
    chmod +x ./DTool
    # 清掉上一轮回退可能留下的失败版本残留。
    rm -f ./DTool.failed
fi

# ---- 自更新闭环：新版连续启动失败时自动回退到上一版 ----
FAIL_THRESHOLD=3
FAIL_FILE=./.start-fail-count
child=""

on_term() {
    # docker stop 发来的 SIGTERM 转发给子进程，让它优雅退出。
    if [ -n "$child" ]; then
        kill -TERM "$child" 2>/dev/null
    fi
}
trap on_term TERM INT

while true; do
    start_ts=$(date +%s)
    ./DTool &
    child=$!
    wait "$child"
    code=$?
    child=""
    elapsed=$(( $(date +%s) - start_ts ))

    # 以下情况不算启动失败：
    #   - 正常退出（0）；自更新成功就是 exit 0，之后由容器 restart 策略拉起；
    #   - 被 SIGTERM 终止（143）：docker stop 的优雅停止；
    #   - 已稳定运行超过 30 秒（崩溃循环会在启动后几秒内退出）。
    # 注意 137(SIGKILL) 不在此列：容器内子进程收到 137 基本只有 OOM 一种来源，
    # 那正是最该回退到上一版的情况（旧实现把它当"稳定"，导致回退永不触发）。
    if [ "$elapsed" -ge 30 ] || [ "$code" -eq 0 ] || [ "$code" -eq 143 ]; then
        rm -f "$FAIL_FILE"
        break
    fi

    fails=$(cat "$FAIL_FILE" 2>/dev/null || echo 0)
    fails=$((fails + 1))
    echo "$fails" > "$FAIL_FILE"
    echo "DTool exited with code $code after ${elapsed}s (fail $fails/$FAIL_THRESHOLD)" >&2

    if [ "$fails" -ge "$FAIL_THRESHOLD" ] && [ -f ./DTool.bak ]; then
        echo "rolling back to previous binary" >&2
        mv ./DTool ./DTool.failed
        mv ./DTool.bak ./DTool
        chmod +x ./DTool
        rm -f "$FAIL_FILE"
    fi
    sleep 2
done
