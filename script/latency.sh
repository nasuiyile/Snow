#!/bin/sh

# 检查是否提供了至少一个参数
if [ "$#" -lt 1 ]; then
    echo "用法: $0 <主机1> <主机2> ... <主机N>"
    exit 1
fi

HOSTS="$@"
NUM_HOSTS=$#

# 设置退出时清理
trap 'echo "\n监控结束"; exit 0' INT TERM

echo ${HOSTS}




while true; do
    # 使用普通变量存储结果
    current_ms=""

    # 对每个主机进行ping测试
    i=0
    for host in $HOSTS; do
        ping_result=$(ping -c 1 "$host" | grep 'time=' | cut -d '=' -f 4 | cut -d ' ' -f 1 2>/dev/null)

        if [ -z "$ping_result" ]; then
            current_ms="$current_ms 超时      "
        else
            current_ms="$current_ms ${ping_result}ms     "
        fi
        i=$((i + 1))
    done

    # 输出结果
    echo "$current_ms"

    # 等待1秒
    sleep 1
done