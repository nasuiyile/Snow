#!/bin/bash

# 检查是否提供了至少一个参数
if [ "$#" -lt 1 ]; then
    echo "用法: $0 <主机1> <主机2> ... <主机N>"
    exit 1
fi

HOSTS=("$@")
NUM_HOSTS=$#

# 初始化统计变量数组
counts=()
totals=()
for ((i=0; i<NUM_HOSTS; i++)); do
    counts[i]=0
    totals[i]=0
done

# 设置退出时清理
trap 'echo -e "\n监控结束"; exit 0' INT TERM

echo "开始监控 ${HOSTS[@]} 的延迟..."
echo "按 Ctrl+C 停止监控"
echo "--------------------------------------------------"

# 打印表头
header=""
for host in "${HOSTS[@]}"; do
    header+="当前延迟($host) 平均延迟($host)  "
done
echo "$header"
echo "--------------------------------------------------"

while true; do
    # 存储当前循环的结果
    current_ms=()
    avg_ms=()

    # 对每个主机进行ping测试
    for ((i=0; i<NUM_HOSTS; i++)); do
        ping_result=$(ping -c 1 "${HOSTS[i]}" | grep 'time=' | cut -d '=' -f 4 | cut -d ' ' -f 1)

        if [ -z "$ping_result" ]; then
            current_ms[i]="超时    "
        else
            current_ms[i]="${ping_result}ms "
            totals[i]=$(echo "${totals[i]} + $ping_result" | bc)
            counts[i]=$((counts[i] + 1))
            avg=$(printf "%.2f" $(echo "scale=2; ${totals[i]} / ${counts[i]}" | bc))
            avg_ms[i]="${avg}ms "

        fi
    done

    # 输出结果
    line=""
    for ((i=0; i<NUM_HOSTS; i++)); do
        line+="${current_ms[i]} ${avg_ms[i]:-"N/A    "} "
    done
    echo "$line"

    # 等待1秒
    sleep 1
done